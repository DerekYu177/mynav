package core

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/GianlucaP106/gotmux/gotmux"
)

// MynavWorktreeOption is the tmux user-option key that marks a session
// as worktree-backed. The reconciler sets it on session creation and
// reads it to decide which sessions are mynav-managed. Sessions
// without this option are invisible to the worktree-sync feature.
const MynavWorktreeOption = "@mynav-worktree"

// Worktree is a directory directly under the configured worktree root
// that contains a `.git` entry — i.e. a real git worktree that should
// have a tmux session.
type Worktree struct {
	Name string // basename
	Path string // absolute
}

// ManagedSession is a tmux session whose @mynav-worktree option is
// non-empty. mynav only ever acts on these.
type ManagedSession struct {
	Name       string
	MarkerPath string
}

// IsPending reports whether the marker path no longer resolves to a
// git worktree on disk. Pending sessions are rendered dimmed in the
// grid; mynav never kills them — that's the user's call.
func (m ManagedSession) IsPending() bool {
	if m.MarkerPath == "" {
		return false
	}
	return !isWorktreeDir(m.MarkerPath)
}

// listWorktrees returns directories directly under root that look
// like git worktrees, sorted by name. Missing root is treated as
// "no worktrees" so a misconfigured field doesn't crash the goroutine.
func listWorktrees(root string) ([]Worktree, error) {
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Worktree, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(root, e.Name())
		if !isWorktreeDir(p) {
			continue
		}
		out = append(out, Worktree{Name: e.Name(), Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// isWorktreeDir is true iff path/.git exists. Linked worktrees have
// `.git` as a file pointing at the main repo's worktrees dir; the
// main repo has it as a directory. Both are valid here.
func isWorktreeDir(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
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
		if err != nil || opt == nil || opt.Value == "" {
			continue
		}
		out = append(out, ManagedSession{Name: s.Name, MarkerPath: opt.Value})
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
// directly under root. It only ever creates sessions; it never kills
// them. When a worktree disappears, the matching session enters the
// pending state and waits for the user to clean it up manually.
type Reconciler struct {
	client tmuxClient
	root   string
}

// NewReconciler wires up a Reconciler against the real tmux server.
func NewReconciler(t *gotmux.Tmux, root string) *Reconciler {
	return &Reconciler{client: &gotmuxClient{t: t}, root: root}
}

// Root returns the worktree root the reconciler watches, or "" when
// the feature is disabled.
func (r *Reconciler) Root() string { return r.root }

// ManagedSessions returns the current managed-session set as seen by
// the reconciler — used by the UI render to detect pending state.
func (r *Reconciler) ManagedSessions() ([]ManagedSession, error) {
	return r.client.listManaged()
}

// Tick runs one reconciliation pass: any worktree without a session
// gets one. A failure on any single worktree aborts the pass; the
// next tick will retry. No-op when root is unset.
func (r *Reconciler) Tick() error {
	if r.root == "" {
		return nil
	}
	wts, err := listWorktrees(r.root)
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
	for _, w := range wts {
		if _, ok := have[w.Path]; ok {
			continue
		}
		if err := r.client.createManaged(w.Name, w.Path); err != nil {
			return err
		}
	}
	return nil
}
