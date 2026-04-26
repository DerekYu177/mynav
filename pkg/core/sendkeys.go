package core

import "os/exec"

// SendKeys shells out to `tmux send-keys -t <target>` with the given keys.
// gotmux does not expose send-keys, so we invoke tmux directly.
//
// `target` is typically a pane id (e.g. "%17") or session:window.pane string.
// Each entry in `keys` is passed as a separate argument so tmux can interpret
// named keys like "Up", "Enter", or literal text.
func SendKeys(target string, keys ...string) error {
	if target == "" || len(keys) == 0 {
		return nil
	}
	args := append([]string{"send-keys", "-t", target}, keys...)
	return exec.Command("tmux", args...).Run()
}
