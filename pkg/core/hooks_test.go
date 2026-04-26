package core

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestStatusFromEvent(t *testing.T) {
	cases := []struct {
		name  string
		event HookEvent
		want  ClaudeStatus
	}{
		{"SessionStart -> Running", HookEvent{Event: "SessionStart"}, ClaudeRunning},
		{"PreToolUse -> Running", HookEvent{Event: "PreToolUse"}, ClaudeRunning},
		{"UserPromptSubmit -> Running", HookEvent{Event: "UserPromptSubmit"}, ClaudeRunning},
		{"Notification permission_prompt -> NeedsInput", HookEvent{Event: "Notification", NotificationType: "permission_prompt"}, ClaudeNeedsInput},
		{"Notification idle -> Idle", HookEvent{Event: "Notification", NotificationType: "session_idle"}, ClaudeIdle},
		{"PostToolUse -> Running", HookEvent{Event: "PostToolUse"}, ClaudeRunning},
		{"Stop -> Idle", HookEvent{Event: "Stop"}, ClaudeIdle},
		{"Stop interrupt -> Error", HookEvent{Event: "Stop", IsInterrupt: true}, ClaudeError},
		{"SessionEnd -> Dead", HookEvent{Event: "SessionEnd"}, ClaudeDead},
		{"unknown event -> Idle", HookEvent{Event: "WhateverNew"}, ClaudeIdle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusFromEvent(c.event); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestHookStoreStatusEmptyPaneID(t *testing.T) {
	s := NewHookStore()
	if _, ok := s.Status(""); ok {
		t.Fatal("empty pane id should not resolve")
	}
}

func TestHookStoreStatusUnknownPane(t *testing.T) {
	s := NewHookStore()
	if _, ok := s.Status("%nope"); ok {
		t.Fatal("unknown pane should not resolve")
	}
}

func TestHookStoreRecordKeepsNewest(t *testing.T) {
	s := NewHookStore()
	now := time.Now()
	s.Record(HookEvent{PaneID: "%1", Event: "PreToolUse", Timestamp: now})
	s.Record(HookEvent{PaneID: "%1", Event: "Stop", Timestamp: now.Add(time.Second)})
	// out-of-order arrival shouldn't roll back
	s.Record(HookEvent{PaneID: "%1", Event: "PreToolUse", Timestamp: now.Add(-time.Second)})

	got, ok := s.Status("%1")
	if !ok {
		t.Fatal("expected status")
	}
	if got != ClaudeIdle {
		t.Errorf("got %v, want Idle (newest event was Stop)", got)
	}
}

func TestHookStoreChangedSignals(t *testing.T) {
	s := NewHookStore()
	now := time.Now()

	// First record: Changed should fire.
	s.Record(HookEvent{PaneID: "%1", Event: "PreToolUse", Timestamp: now})
	select {
	case <-s.Changed():
	default:
		t.Fatal("expected Changed signal after first Record")
	}

	// Channel should be drained — no spurious second signal.
	select {
	case <-s.Changed():
		t.Fatal("Changed should be empty after consuming")
	default:
	}

	// Stale (older-than-current) event must NOT signal.
	s.Record(HookEvent{PaneID: "%1", Event: "Stop", Timestamp: now.Add(-time.Second)})
	select {
	case <-s.Changed():
		t.Fatal("stale event should not signal Changed")
	default:
	}

	// Newer event signals again.
	s.Record(HookEvent{PaneID: "%1", Event: "Stop", Timestamp: now.Add(time.Second)})
	select {
	case <-s.Changed():
	default:
		t.Fatal("expected Changed signal after newer Record")
	}
}

func TestHookStoreChangedCoalesces(t *testing.T) {
	s := NewHookStore()
	now := time.Now()
	// Burst of 5 distinct updates with no reader between them — must
	// collapse to a single pending signal.
	for i := 0; i < 5; i++ {
		s.Record(HookEvent{
			PaneID:    "%1",
			Event:     "PreToolUse",
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
		})
	}
	<-s.Changed() // one drain
	select {
	case <-s.Changed():
		t.Fatal("expected exactly one pending signal after burst")
	default:
	}
}

func TestHookStoreStaleEventDropped(t *testing.T) {
	s := NewHookStore()
	s.Record(HookEvent{
		PaneID:    "%1",
		Event:     "PreToolUse",
		Timestamp: time.Now().Add(-2 * hookStaleThreshold),
	})
	if _, ok := s.Status("%1"); ok {
		t.Fatal("stale event should not resolve")
	}
}

func TestWriteHookEventRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%17")
	fixedTime := time.Date(2026, 4, 26, 17, 0, 0, 123, time.UTC)

	payload := `{"session_id":"abc","cwd":"/x","tool_name":"Bash","notification_type":"permission_prompt"}`
	if err := writeHookEvent(dir, "Notification", strings.NewReader(payload), func() time.Time { return fixedTime }); err != nil {
		t.Fatalf("writeHookEvent: %v", err)
	}

	store := NewHookStore()
	if err := store.Drain(dir); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	// Drain skips the stale check (it just records); Status applies it.
	// We just-wrote with a time well in the past for the real clock, so
	// poke the latest map directly to verify event content.
	store.mu.Lock()
	got, ok := store.latest["%17"]
	store.mu.Unlock()

	if !ok {
		t.Fatal("expected pane %17 in store")
	}
	if got.Event != "Notification" {
		t.Errorf("Event = %q, want Notification", got.Event)
	}
	if got.NotificationType != "permission_prompt" {
		t.Errorf("NotificationType = %q, want permission_prompt", got.NotificationType)
	}
	if got.SessionID != "abc" {
		t.Errorf("SessionID = %q, want abc", got.SessionID)
	}
	if got.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want Bash", got.ToolName)
	}
	if !got.Timestamp.Equal(fixedTime) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, fixedTime)
	}
}

func TestDrainConsumesQueueFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMUX_PANE", "%1")
	now := time.Now()

	if err := writeHookEvent(dir, "SessionStart", strings.NewReader(`{}`), func() time.Time { return now }); err != nil {
		t.Fatal(err)
	}
	if err := writeHookEvent(dir, "Stop", strings.NewReader(`{}`), func() time.Time { return now.Add(time.Millisecond) }); err != nil {
		t.Fatal(err)
	}

	store := NewHookStore()
	if err := store.Drain(dir); err != nil {
		t.Fatal(err)
	}

	got, ok := store.Status("%1")
	if !ok {
		t.Fatal("expected status after drain")
	}
	if got != ClaudeIdle {
		t.Errorf("got %v, want Idle (last event was Stop)", got)
	}

	// Drain should have consumed the queue files.
	leftovers, err := readQueueDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) != 0 {
		t.Errorf("queue not drained: %v", leftovers)
	}

	// A second drain on an already-empty dir is a no-op (no error).
	if err := store.Drain(dir); err != nil {
		t.Errorf("second Drain returned error: %v", err)
	}
}

func TestDrainOnMissingDir(t *testing.T) {
	store := NewHookStore()
	if err := store.Drain(t.TempDir() + "/does-not-exist"); err != nil {
		t.Errorf("Drain on missing dir should be a no-op, got %v", err)
	}
}

// readQueueDir returns the names of q-*.json files in dir.
func readQueueDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "q-") && strings.HasSuffix(n, ".json") {
			out = append(out, n)
		}
	}
	return out, nil
}
