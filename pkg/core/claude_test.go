package core

import "testing"

func TestDetectClaudeStatus(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want ClaudeStatus
	}{
		{
			name: "empty pane is dead",
			pane: "",
			want: ClaudeDead,
		},
		{
			name: "shell prompt without claude is dead",
			pane: "user@host:~/project$ ls\nfoo bar\nuser@host:~/project$ ",
			want: ClaudeDead,
		},
		{
			name: "esc to interrupt means running",
			pane: "╭─────────╮\n│ > query │\n╰─────────╯\n✻ Cogitating… (esc to interrupt)",
			want: ClaudeRunning,
		},
		{
			name: "API Error means error",
			pane: "API Error: 500 Internal Server Error\n╭─╮\n│ > │\n╰─╯",
			want: ClaudeError,
		},
		{
			name: "numbered options means needs input",
			pane: "Do you want to make this edit?\n 1. Yes\n 2. Yes, and don't ask again\n 3. No, and tell Claude\n",
			want: ClaudeNeedsInput,
		},
		{
			name: "selector arrow means needs input",
			pane: "Pick a model:\n  Sonnet\n❯ Opus\n  Haiku\n",
			want: ClaudeNeedsInput,
		},
		{
			name: "claude prompt box alone is idle",
			pane: "╭─────────────────╮\n│ >                │\n╰─────────────────╯",
			want: ClaudeIdle,
		},
		{
			name: "running takes priority over needs input",
			pane: "Do you want to do this?\n 1. Yes\n 2. No\nThinking… (esc to interrupt)",
			want: ClaudeRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectClaudeStatus(tt.pane)
			if got != tt.want {
				t.Errorf("DetectClaudeStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectApprovalMode(t *testing.T) {
	tests := []struct {
		name string
		pane string
		want ApprovalMode
	}{
		{
			name: "numbered list is one-key",
			pane: "Do you want to make this edit?\n 1. Yes\n 2. Yes, and don't ask again\n 3. No",
			want: ApprovalOneKey,
		},
		{
			name: "selector arrow is selector",
			pane: "Pick a model:\n  Sonnet\n❯ Opus\n  Haiku",
			want: ApprovalSelector,
		},
		{
			name: "neither is none",
			pane: "What is your name? │ > │",
			want: ApprovalNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectApprovalMode(tt.pane)
			if got != tt.want {
				t.Errorf("DetectApprovalMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
