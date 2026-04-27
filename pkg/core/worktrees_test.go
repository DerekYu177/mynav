package core

import (
	"errors"
	"os"
	"path/filepath"
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

// makeWorktree creates a fake worktree at root/name by writing a .git
// file with the linked-worktree gitfile syntax. Mirrors what `git
// worktree add` does for a linked worktree.
func makeWorktree(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, ".git"), []byte("gitdir: ../.git/worktrees/"+name), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListWorktreesEmptyRoot(t *testing.T) {
	out, err := listWorktrees("")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("got %v", out)
	}
}

func TestListWorktreesMissingRoot(t *testing.T) {
	out, err := listWorktrees(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("got %v", out)
	}
}

func TestListWorktreesSkipsNonWorktrees(t *testing.T) {
	root := t.TempDir()
	makeWorktree(t, root, "real")
	if err := os.MkdirAll(filepath.Join(root, "notawt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := listWorktrees(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Name != "real" {
		t.Fatalf("got %v", out)
	}
}

func TestListWorktreesSorted(t *testing.T) {
	root := t.TempDir()
	makeWorktree(t, root, "charlie")
	makeWorktree(t, root, "alpha")
	makeWorktree(t, root, "bravo")
	out, err := listWorktrees(root)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{out[0].Name, out[1].Name, out[2].Name}
	want := []string{"alpha", "bravo", "charlie"}
	for i := range names {
		if names[i] != want[i] {
			t.Errorf("order: got %v, want %v", names, want)
		}
	}
}

func TestIsPending(t *testing.T) {
	root := t.TempDir()
	live := makeWorktree(t, root, "live")
	tests := []struct {
		name string
		s    ManagedSession
		want bool
	}{
		{"live worktree not pending", ManagedSession{Name: "live", MarkerPath: live}, false},
		{"missing worktree pending", ManagedSession{Name: "ghost", MarkerPath: filepath.Join(root, "ghost")}, true},
		{"unmarked session not pending", ManagedSession{Name: "x"}, false},
	}
	for _, c := range tests {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.IsPending(); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestReconcileCreatesMissingSessions(t *testing.T) {
	root := t.TempDir()
	a := makeWorktree(t, root, "alpha")
	b := makeWorktree(t, root, "bravo")

	fake := &fakeTmux{
		sessions: []ManagedSession{{Name: "alpha", MarkerPath: a}},
	}
	r := &Reconciler{client: fake, root: root}
	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}

	if len(fake.creates) != 1 {
		t.Fatalf("expected 1 create, got %d: %v", len(fake.creates), fake.creates)
	}
	if fake.creates[0].Name != "bravo" || fake.creates[0].MarkerPath != b {
		t.Errorf("got %+v, want bravo @ %s", fake.creates[0], b)
	}
}

func TestReconcileNeverKills(t *testing.T) {
	root := t.TempDir()
	// No worktrees on disk, but a managed session points at one.
	ghost := filepath.Join(root, "ghost")
	fake := &fakeTmux{
		sessions: []ManagedSession{{Name: "ghost", MarkerPath: ghost}},
	}
	r := &Reconciler{client: fake, root: root}
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

func TestReconcileEmptyRootIsNoOp(t *testing.T) {
	fake := &fakeTmux{}
	r := &Reconciler{client: fake, root: ""}
	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(fake.creates) != 0 {
		t.Errorf("disabled feature should not create anything: %v", fake.creates)
	}
}

func TestReconcileIdempotent(t *testing.T) {
	root := t.TempDir()
	a := makeWorktree(t, root, "alpha")
	fake := &fakeTmux{}
	r := &Reconciler{client: fake, root: root}

	if err := r.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(fake.creates) != 1 || fake.creates[0].MarkerPath != a {
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
	root := t.TempDir()
	makeWorktree(t, root, "alpha")
	makeWorktree(t, root, "bravo")
	want := errors.New("boom")
	fake := &fakeTmux{
		createFn: func(name, _ string) error {
			if name == "alpha" {
				return want
			}
			return nil
		},
	}
	r := &Reconciler{client: fake, root: root}
	if err := r.Tick(); !errors.Is(err, want) {
		t.Errorf("got %v, want %v", err, want)
	}
	// alpha attempted, bravo skipped — next tick will retry.
	if len(fake.creates) != 0 {
		t.Errorf("create should have failed before recording: %v", fake.creates)
	}
}
