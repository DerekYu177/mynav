package core

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// hookEvents are the Claude Code hook events mynav writes entries for
// in settings.json. The set is intentionally narrow — these are enough
// to drive every ClaudeStatus transition statusFromEvent recognises.
var hookEvents = []string{
	"PreToolUse",
	"Notification",
	"Stop",
	"SessionStart",
	"SessionEnd",
}

// InstallHooks merges mynav's hook entries into Claude Code's
// settings.json. Calling it twice is a no-op; existing entries from
// other tools are preserved.
func InstallHooks(binPath string) error {
	return mutateClaudeSettings(func(s map[string]any) {
		installHooks(s, binPath)
	})
}

// UninstallHooks removes only the mynav-managed hook entries from
// settings.json. Other entries — including non-mynav hooks under the
// same events — are left alone.
func UninstallHooks() error {
	return mutateClaudeSettings(uninstallHooks)
}

func installHooks(settings map[string]any, binPath string) {
	hooks := getOrCreateMap(settings, "hooks")
	for _, ev := range hookEvents {
		entries := filterEntries(toEntrySlice(hooks[ev]), isMynavManaged)
		entries = append(entries, map[string]any{
			"matcher": "*",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": fmt.Sprintf("%s hook %s", binPath, ev),
				},
			},
		})
		hooks[ev] = entries
	}
}

func uninstallHooks(settings map[string]any) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return
	}
	for _, ev := range hookEvents {
		entries := filterEntries(toEntrySlice(hooks[ev]), isMynavManaged)
		if len(entries) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = entries
		}
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}
}

// isMynavManaged identifies an entry as mynav-owned by the shape of
// its command line ("<bin>/mynav hook <Event>"). The match deliberately
// allows the binary path to vary so reinstalling from a different
// location doesn't strand orphan entries.
func isMynavManaged(entry map[string]any) bool {
	inner, _ := entry["hooks"].([]any)
	for _, h := range inner {
		m, _ := h.(map[string]any)
		if m == nil {
			continue
		}
		if t, _ := m["type"].(string); t != "command" {
			continue
		}
		cmd, _ := m["command"].(string)
		if strings.Contains(cmd, "mynav hook ") {
			return true
		}
	}
	return false
}

func toEntrySlice(v any) []map[string]any {
	arr, _ := v.([]any)
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func filterEntries(entries []map[string]any, drop func(map[string]any) bool) []any {
	out := make([]any, 0, len(entries))
	for _, e := range entries {
		if drop(e) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func getOrCreateMap(parent map[string]any, key string) map[string]any {
	if m, ok := parent[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	parent[key] = m
	return m
}

// ClaudeSettingsPath returns the path to Claude Code's settings.json.
// CLAUDE_HOME wins if set (used by tests); otherwise it's
// $HOME/.claude/settings.json.
func ClaudeSettingsPath() (string, error) {
	if h := os.Getenv("CLAUDE_HOME"); h != "" {
		return filepath.Join(h, "settings.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

func mutateClaudeSettings(fn func(map[string]any)) error {
	path, err := ClaudeSettingsPath()
	if err != nil {
		return err
	}
	settings, err := readClaudeSettings(path)
	if err != nil {
		return err
	}
	fn(settings)
	return writeClaudeSettings(path, settings)
}

func readClaudeSettings(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func writeClaudeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), "settings-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}
