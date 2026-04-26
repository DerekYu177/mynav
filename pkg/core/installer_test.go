package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupClaudeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_HOME", dir)
	return dir
}

func readSettings(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	return s
}

func TestInstallHooksOnFreshHome(t *testing.T) {
	home := setupClaudeHome(t)

	if err := InstallHooks("/path/to/mynav"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	settings := readSettings(t, filepath.Join(home, "settings.json"))
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks key missing or wrong type: %T", settings["hooks"])
	}

	for _, ev := range hookEvents {
		entries, ok := hooks[ev].([]any)
		if !ok || len(entries) != 1 {
			t.Errorf("event %q: expected 1 entry, got %v", ev, hooks[ev])
			continue
		}
		entry := entries[0].(map[string]any)
		if entry["matcher"] != "*" {
			t.Errorf("event %q: matcher = %v, want *", ev, entry["matcher"])
		}
		inner := entry["hooks"].([]any)
		cmd := inner[0].(map[string]any)["command"].(string)
		want := "/path/to/mynav hook " + ev
		if cmd != want {
			t.Errorf("event %q: command = %q, want %q", ev, cmd, want)
		}
	}
}

func TestInstallHooksIsIdempotent(t *testing.T) {
	setupClaudeHome(t)

	for i := 0; i < 3; i++ {
		if err := InstallHooks("/p/mynav"); err != nil {
			t.Fatalf("InstallHooks #%d: %v", i, err)
		}
	}

	path, _ := ClaudeSettingsPath()
	settings := readSettings(t, path)
	hooks := settings["hooks"].(map[string]any)
	for _, ev := range hookEvents {
		entries := hooks[ev].([]any)
		if len(entries) != 1 {
			t.Errorf("event %q: %d entries after 3 installs, want 1", ev, len(entries))
		}
	}
}

func TestInstallHooksPreservesOtherSettings(t *testing.T) {
	home := setupClaudeHome(t)

	// Pre-seed a settings.json with unrelated keys and a non-mynav hook.
	pre := map[string]any{
		"alwaysThinkingEnabled": true,
		"model":                 "opus",
		"env": map[string]any{
			"FOO": "bar",
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "rtk hook claude",
						},
					},
				},
			},
		},
	}
	raw, _ := json.MarshalIndent(pre, "", "  ")
	if err := os.WriteFile(filepath.Join(home, "settings.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallHooks("/p/mynav"); err != nil {
		t.Fatalf("InstallHooks: %v", err)
	}

	got := readSettings(t, filepath.Join(home, "settings.json"))
	if got["alwaysThinkingEnabled"] != true {
		t.Errorf("alwaysThinkingEnabled lost")
	}
	if got["model"] != "opus" {
		t.Errorf("model lost")
	}
	envMap, _ := got["env"].(map[string]any)
	if envMap["FOO"] != "bar" {
		t.Errorf("env.FOO lost")
	}

	// PreToolUse should now have 2 entries: the user's rtk hook plus mynav's.
	pre2 := got["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(pre2) != 2 {
		t.Fatalf("PreToolUse: %d entries, want 2", len(pre2))
	}
	rtkSurvived := false
	mynavPresent := false
	for _, e := range pre2 {
		entry := e.(map[string]any)
		inner := entry["hooks"].([]any)
		cmd := inner[0].(map[string]any)["command"].(string)
		if cmd == "rtk hook claude" {
			rtkSurvived = true
		}
		if cmd == "/p/mynav hook PreToolUse" {
			mynavPresent = true
		}
	}
	if !rtkSurvived {
		t.Error("rtk hook was removed")
	}
	if !mynavPresent {
		t.Error("mynav hook not added")
	}
}

func TestUninstallHooksLeavesOthersIntact(t *testing.T) {
	home := setupClaudeHome(t)

	pre := map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{"type": "command", "command": "rtk hook claude"},
					},
				},
			},
		},
	}
	raw, _ := json.MarshalIndent(pre, "", "  ")
	os.WriteFile(filepath.Join(home, "settings.json"), raw, 0o644)

	if err := InstallHooks("/p/mynav"); err != nil {
		t.Fatal(err)
	}
	if err := UninstallHooks(); err != nil {
		t.Fatal(err)
	}

	got := readSettings(t, filepath.Join(home, "settings.json"))
	if got["model"] != "opus" {
		t.Error("model lost")
	}
	hooks := got["hooks"].(map[string]any)

	pre2, _ := hooks["PreToolUse"].([]any)
	if len(pre2) != 1 {
		t.Fatalf("PreToolUse: %d entries after uninstall, want 1 (the rtk hook)", len(pre2))
	}
	entry := pre2[0].(map[string]any)
	cmd := entry["hooks"].([]any)[0].(map[string]any)["command"].(string)
	if cmd != "rtk hook claude" {
		t.Errorf("PreToolUse[0].command = %q, want rtk hook claude", cmd)
	}

	for _, ev := range []string{"Notification", "Stop", "SessionStart", "SessionEnd"} {
		if _, present := hooks[ev]; present {
			t.Errorf("event %q still present after uninstall", ev)
		}
	}
}

func TestUninstallHooksOnFreshHome(t *testing.T) {
	setupClaudeHome(t)
	if err := UninstallHooks(); err != nil {
		t.Errorf("UninstallHooks on fresh home should be a no-op, got %v", err)
	}
}
