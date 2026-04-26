package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// HookEvent is the on-disk representation of a single Claude Code hook
// fire. Written by the `mynav hook` subcommand (one file per event) and
// read by HookStore.Drain on the mynav side.
type HookEvent struct {
	PaneID           string    `json:"pane_id"`
	SessionID        string    `json:"session_id"`
	CWD              string    `json:"cwd"`
	Event            string    `json:"event"`
	NotificationType string    `json:"notification_type,omitempty"`
	StopReason       string    `json:"stop_reason,omitempty"`
	IsInterrupt      bool      `json:"is_interrupt,omitempty"`
	ToolName         string    `json:"tool_name,omitempty"`
	Timestamp        time.Time `json:"ts"`
}

// hookStaleThreshold is how long a hook event remains authoritative
// after it lands. After this, HookStore.Status reports nothing and
// callers can fall back to pattern matching.
const hookStaleThreshold = 5 * time.Minute

// QueueDir returns the directory mynav writes hook events to. Falls
// back to /tmp/mynav/queue when XDG_RUNTIME_DIR is unset (macOS doesn't
// set it).
func QueueDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "mynav", "queue")
	}
	return filepath.Join(os.TempDir(), "mynav", "queue")
}

// statusFromEvent maps a hook event to a ClaudeStatus.
func statusFromEvent(e HookEvent) ClaudeStatus {
	switch e.Event {
	case "SessionStart", "UserPromptSubmit", "PreToolUse":
		return ClaudeRunning
	case "Notification":
		if e.NotificationType == "permission_prompt" {
			return ClaudeNeedsInput
		}
		return ClaudeIdle
	case "Stop", "PostToolUse":
		if e.IsInterrupt {
			return ClaudeError
		}
		return ClaudeIdle
	case "SessionEnd":
		return ClaudeDead
	}
	return ClaudeIdle
}

// WriteHookEvent reads a Claude Code hook payload (JSON on stdin),
// enriches it with TMUX_PANE from the environment, and writes it as a
// queue file under QueueDir. Hooks must never block Claude Code, so
// callers should swallow the returned error.
func WriteHookEvent(event string, stdin io.Reader) error {
	return writeHookEvent(QueueDir(), event, stdin, time.Now)
}

// writeHookEvent is the testable inner. It takes the queue dir, event
// name, an io.Reader for the payload, and a clock so tests can pin a
// timestamp. The file is written through os.CreateTemp + Rename so
// readers never see a half-written event.
func writeHookEvent(dir, event string, stdin io.Reader, now func() time.Time) error {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return err
	}
	var payload map[string]any
	_ = json.Unmarshal(raw, &payload) // tolerate empty / malformed input

	getStr := func(k string) string {
		v, _ := payload[k].(string)
		return v
	}
	getBool := func(k string) bool {
		v, _ := payload[k].(bool)
		return v
	}

	ts := now()
	he := HookEvent{
		PaneID:           os.Getenv("TMUX_PANE"),
		SessionID:        getStr("session_id"),
		CWD:              getStr("cwd"),
		Event:            event,
		NotificationType: getStr("notification_type"),
		StopReason:       getStr("stop_reason"),
		IsInterrupt:      getBool("is_interrupt"),
		ToolName:         getStr("tool_name"),
		Timestamp:        ts,
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	final := filepath.Join(dir, fmt.Sprintf("q-%020d.json", ts.UnixNano()))
	tmp, err := os.CreateTemp(dir, "tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	enc := json.NewEncoder(tmp)
	if err := enc.Encode(he); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, final); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// HookStore holds the most recent hook event per pane id and resolves
// it to a ClaudeStatus.
type HookStore struct {
	mu     sync.Mutex
	latest map[string]HookEvent
}

func NewHookStore() *HookStore {
	return &HookStore{latest: map[string]HookEvent{}}
}

// Status returns the cached state for a pane id, or false if nothing
// is recorded (or the most recent event is past the stale threshold).
func (s *HookStore) Status(paneID string) (ClaudeStatus, bool) {
	if paneID == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.latest[paneID]
	if !ok {
		return 0, false
	}
	if time.Since(e.Timestamp) > hookStaleThreshold {
		return 0, false
	}
	return statusFromEvent(e), true
}

// Record applies a single event to the store, keeping only the newest
// event per pane id.
func (s *HookStore) Record(e HookEvent) {
	if e.PaneID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.latest[e.PaneID]
	if !ok || e.Timestamp.After(cur.Timestamp) {
		s.latest[e.PaneID] = e
	}
}

// Drain reads and removes every queue file from dir, applying each to
// the store. Files are processed in filename order (chronological
// because of the q-<unix-nanos>.json naming convention).
func (s *HookStore) Drain(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if !strings.HasPrefix(n, "q-") || !strings.HasSuffix(n, ".json") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := filepath.Join(dir, n)
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		var he HookEvent
		if err := json.NewDecoder(f).Decode(&he); err == nil {
			s.Record(he)
		}
		f.Close()
		os.Remove(p)
	}
	return nil
}
