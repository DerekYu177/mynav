package core

import (
	"errors"
	"testing"
)

type fakeTmux struct {
	sessions []ManagedSession
	creates  []ManagedSession
	createFn func(name, path string) error
}

func (f *fakeTmux) listManaged() ([]ManagedSession, error) { return f.sessions, nil }
func (f *fakeTmux) createManaged(name, path string) error {
	if f.createFn != nil {
		if err := f.createFn(name, path); err != nil {
			return err
		}
	}
	s := ManagedSession{Name: name, MarkerPath: path}
	f.sessions = append(f.sessions, s)
	f.creates = append(f.creates, s)
	return nil
}

func TestListWorktreesEmptyCmd(t *testing.T) {
	out, err := listWorktrees("")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("got %v", out)
	}
}

func TestListWorktreesParsesTabSeparated(t *testing.T) {
	out, err := listWorktrees(`printf 'alpha\t/a\nbravo\t/b\n'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("got %v", out)
	}
	if out[0] != (Worktree{Name: "alpha", Path: "/a"}) {
		t.Errorf("got %+v", out[0])
	}
	if out[1] != (Worktree{Name: "bravo", Path: "/b"}) {
		t.Errorf("got %+v", out[1])
	}
}

func TestListWorktreesSkipsBadLines(t *testing.T) {
	// Tolerate blank lines, single-field lines, and lines with empty
	// fields — the wrapper might emit headers or stray whitespace.
	out, err := listWorktrees(`printf 'header_only\nalpha\t/a\n\nlone\n\t/onlypath\nname\t\n'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "alpha" || out[0].Path != "/a" {
		t.Fatalf("got %v", out)
	}
}

func TestListWorktreesCommandError(t *testing.T) {
	_, err := listWorktrees(`exit 1`)
	if err == nil {
		t.Fatal("expected error from failing cmd")
	}
}

func TestIsPending(t *testing.T) {
	live := map[string]struct{}{"/a": {}}
	tests := []struct {
		name string
		s    ManagedSession
		want bool
	}{
		{"in live set not pending", ManagedSession{Name: "live", MarkerPath: "/a"}, false},
		{"absent path pending", ManagedSession{Name: "ghost", MarkerPath: "/b"}, true},
		{"unmarked not pending", ManagedSession{Name: "x"}, false},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.IsPending(live); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestReconcileCreatesMissingSessions(t *testing.T) {
	cmd := `printf 'alpha\t/a\nbravo\t/b\n'`
	fake := &fakeTmux{
		sessions: []ManagedSession{{Name: "alpha", MarkerPath: "/a"}},
	}
	r := &Reconciler{client: fake, cmd: cmd}
	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}

	if len(fake.creates) != 1 {
		t.Fatalf("expected 1 create, got %d: %v", len(fake.creates), fake.creates)
	}
	if fake.creates[0].Name != "bravo" || fake.creates[0].MarkerPath != "/b" {
		t.Errorf("got %+v, want bravo @ /b", fake.creates[0])
	}
}

func TestReconcileNeverKills(t *testing.T) {
	// Listing emits nothing, but a managed session points at a path
	// that no longer appears. Tick must not remove it.
	fake := &fakeTmux{
		sessions: []ManagedSession{{Name: "ghost", MarkerPath: "/ghost"}},
	}
	r := &Reconciler{client: fake, cmd: `printf ''`}
	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(fake.creates) != 0 {
		t.Errorf("Tick should not create anything: %v", fake.creates)
	}
	if len(fake.sessions) != 1 {
		t.Errorf("Tick must never remove sessions; got %v", fake.sessions)
	}
}

func TestReconcileEmptyCmdIsNoOp(t *testing.T) {
	fake := &fakeTmux{}
	r := &Reconciler{client: fake, cmd: ""}
	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(fake.creates) != 0 {
		t.Errorf("disabled feature should not create anything: %v", fake.creates)
	}
	if _, ready := r.LiveMarkers(); ready {
		t.Errorf("disabled feature should never become ready")
	}
}

func TestReconcileIdempotent(t *testing.T) {
	cmd := `printf 'alpha\t/a\n'`
	fake := &fakeTmux{}
	r := &Reconciler{client: fake, cmd: cmd}

	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(fake.creates) != 1 || fake.creates[0].MarkerPath != "/a" {
		t.Fatalf("first tick: got %v", fake.creates)
	}

	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(fake.creates) != 1 {
		t.Errorf("second tick should be no-op, total creates: %d", len(fake.creates))
	}
}

func TestReconcileCreateErrorAborts(t *testing.T) {
	cmd := `printf 'alpha\t/a\nbravo\t/b\n'`
	want := errors.New("boom")
	fake := &fakeTmux{
		createFn: func(name, _ string) error {
			if name == "alpha" {
				return want
			}
			return nil
		},
	}
	r := &Reconciler{client: fake, cmd: cmd}
	if err := r.Tick(); !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
	if len(fake.creates) != 0 {
		t.Errorf("create should have failed before recording: %v", fake.creates)
	}
	// Failed Tick should not flip to ready — stale live set would
	// produce wrong pending-state on the next render.
	if _, ready := r.LiveMarkers(); ready {
		t.Errorf("failed Tick must not mark reconciler ready")
	}
}

func TestLiveMarkersUpdatedAfterTick(t *testing.T) {
	cmd := `printf 'alpha\t/a\nbravo\t/b\n'`
	fake := &fakeTmux{}
	r := &Reconciler{client: fake, cmd: cmd}

	if _, ready := r.LiveMarkers(); ready {
		t.Errorf("reconciler should not be ready before first Tick")
	}

	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}

	live, ready := r.LiveMarkers()
	if !ready {
		t.Fatal("expected ready after successful Tick")
	}
	if _, ok := live["/a"]; !ok {
		t.Errorf("/a missing from live set: %v", live)
	}
	if _, ok := live["/b"]; !ok {
		t.Errorf("/b missing from live set: %v", live)
	}
}
