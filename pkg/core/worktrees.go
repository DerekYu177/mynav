package core

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/GianlucaP106/gotmux/gotmux"
)

// MynavWorktreeOption is the tmux user-option key that marks a session
// as worktree-backed. The reconciler sets it on session creation and
// reads it to decide which sessions are mynav-managed. Sessions
// without this option are invisible to the worktree-sync feature.
const MynavWorktreeOption = "@mynav-worktree"

// Worktree is one entry produced by the user-configured listing
// command: the tmux session name to use, and the absolute path that
// the session's working directory should be set to.
type Worktree struct {
	Name string
	Path string
}

// ManagedSession is a tmux session whose @mynav-worktree option is
// non-empty. mynav only ever acts on these.
type ManagedSession struct {
	Name       string
	MarkerPath string
}

// IsPending reports whether the marker path is absent from the
// supplied live-set. mynav never kills these sessions — pending is a
// render-only state for the user to clean up.
func (m ManagedSession) IsPending(live map[string]struct{}) bool {
	if m.MarkerPath == "" {
		return false
	}
	_, ok := live[m.MarkerPath]
	return !ok
}

// listWorktrees runs the user-configured command via `sh -c` and
// parses each stdout line as `<tmux-name>\t<absolute-path>`. Empty
// cmd returns nil so a misconfigured field is the same as opt-out.
// Lines that don't have exactly two non-empty fields are skipped, so
// the wrapper can emit blanks or headers without breaking us.
func listWorktrees(cmd string) ([]Worktree, error) {
	if cmd == "" {
		return nil, nil
	}
	c := exec.Command("sh", "-c", cmd)
	out, err := c.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("worktree-list-cmd failed: %w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	var wts []Worktree
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if name == "" || path == "" {
			continue
		}
		wts = append(wts, Worktree{Name: name, Path: path})
	}
	return wts, sc.Err()
}

// tmuxClient is the slice of gotmux behavior Reconciler depends on.
// Defined as an interface so tests can run without a real tmux
// server.
type tmuxClient interface {
	listManaged() ([]ManagedSession, error)
	createManaged(name, path string) error
}

type gotmuxClient struct {
	t *gotmux.Tmux
}

func (g *gotmuxClient) listManaged() ([]ManagedSession, error) {
	sessions, err := g.t.ListSessions()
	if err != nil {
		return nil, err
	}
	out := make([]ManagedSession, 0, len(sessions))
	for _, s := range sessions {
		opt, err := s.Option(MynavWorktreeOption)
		// gotmux wraps tmux's "option not set" exit as a generic
		// error; we can't tell that case from a real failure, so
		// we treat any error or empty value as "not managed".
		if err != nil || opt == nil {
			continue
		}
		// gotmux returns the raw command output, which keeps the
		// trailing newline tmux emits. Without trimming we'd compare
		// "<path>\n" against the clean path produced by the listing
		// command and skip every "have" check, creating a duplicate
		// session every tick.
		v := strings.TrimSpace(opt.Value)
		if v == "" {
			continue
		}
		out = append(out, ManagedSession{Name: s.Name, MarkerPath: v})
	}
	return out, nil
}

func (g *gotmuxClient) createManaged(name, path string) error {
	s, err := g.t.NewSession(&gotmux.SessionOptions{
		Name:           name,
		StartDirectory: path,
	})
	if err != nil {
		return err
	}
	return s.SetOption(MynavWorktreeOption, path)
}

// Reconciler ensures one managed tmux session exists per worktree
// reported by the configured listing command. It only ever creates
// sessions; it never kills them. When a worktree disappears from the
// listing the matching session enters the pending state and waits
// for the user to clean it up manually.
type Reconciler struct {
	client tmuxClient
	cmd    string

	mu    sync.RWMutex
	live  map[string]struct{}
	ready bool
}

// NewReconciler wires up a Reconciler against the real tmux server.
func NewReconciler(t *gotmux.Tmux, cmd string) *Reconciler {
	return &Reconciler{client: &gotmuxClient{t: t}, cmd: cmd}
}

// Cmd returns the configured listing command, or "" when the feature
// is disabled.
func (r *Reconciler) Cmd() string { return r.cmd }

// ManagedSessions returns the current managed-session set as seen by
// the reconciler — used by the UI render to detect pending state.
func (r *Reconciler) ManagedSessions() ([]ManagedSession, error) {
	return r.client.listManaged()
}

// LiveMarkers returns a copy of the marker paths the most recent
// successful Tick observed, plus a `ready` flag. When ready is false
// (no Tick has succeeded yet) callers should treat all sessions as
// non-pending — we don't know what's live, and stale dimming is
// worse than no dimming.
func (r *Reconciler) LiveMarkers() (map[string]struct{}, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.ready {
		return nil, false
	}
	out := make(map[string]struct{}, len(r.live))
	for k := range r.live {
		out[k] = struct{}{}
	}
	return out, true
}

// Tick runs one reconciliation pass: any worktree without a session
// gets one. A failure on any single worktree aborts the pass; the
// next tick will retry. No-op when cmd is empty.
func (r *Reconciler) Tick() error {
	if r.cmd == "" {
		return nil
	}
	wts, err := listWorktrees(r.cmd)
	if err != nil {
		return err
	}
	sessions, err := r.client.listManaged()
	if err != nil {
		return err
	}
	have := make(map[string]struct{}, len(sessions))
	for _, s := range sessions {
		have[s.MarkerPath] = struct{}{}
	}
	live := make(map[string]struct{}, len(wts))
	for _, w := range wts {
		live[w.Path] = struct{}{}
		if _, ok := have[w.Path]; ok {
			continue
		}
		if err := r.client.createManaged(w.Name, w.Path); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.live = live
	r.ready = true
	r.mu.Unlock()
	return nil
}
