package core

import (
	"regexp"
	"strings"
)

// ClaudeStatus represents the detected state of Claude Code running in a tmux pane.
type ClaudeStatus int

const (
	ClaudeDead ClaudeStatus = iota
	ClaudeRunning
	ClaudeNeedsInput
	ClaudeIdle
	ClaudeError
)

// Emoji returns a single-glyph indicator for the status.
func (s ClaudeStatus) Emoji() string {
	switch s {
	case ClaudeRunning:
		return "🟢"
	case ClaudeNeedsInput:
		return "🟡"
	case ClaudeIdle:
		return "⚪"
	case ClaudeError:
		return "🔴"
	case ClaudeDead:
		fallthrough
	default:
		return "⚫"
	}
}

// Label returns a short human-readable label for the status.
func (s ClaudeStatus) Label() string {
	switch s {
	case ClaudeRunning:
		return "Running"
	case ClaudeNeedsInput:
		return "Needs Input"
	case ClaudeIdle:
		return "Idle"
	case ClaudeError:
		return "Error"
	case ClaudeDead:
		fallthrough
	default:
		return "Dead"
	}
}

// String returns "<emoji> <label>" suitable for table display.
func (s ClaudeStatus) String() string {
	return s.Emoji() + " " + s.Label()
}

// patterns we look for in pane captures.
var (
	reEscToInterrupt = regexp.MustCompile(`\(esc to interrupt\)`)
	reApiError       = regexp.MustCompile(`(?i)API Error|Error:|✗ Error`)
	reClaudePrompt   = regexp.MustCompile(`╭─+╮|╰─+╯|│\s*>\s*`)
	reNumberedOption = regexp.MustCompile(`(?m)^\s*[1-9]\.\s+\S`)
	reSelectorArrow  = regexp.MustCompile(`❯`)
	reQuestionMark   = regexp.MustCompile(`\?\s*$|\?\s*\n`)
)

// DetectClaudeStatus inspects a captured tmux pane and classifies it.
//
// Detection is best-effort pattern matching against Claude Code's CLI output;
// it is brittle by design and intended to give a useful at-a-glance indicator
// rather than a guaranteed-correct state.
func DetectClaudeStatus(pane string) ClaudeStatus {
	trimmed := strings.TrimSpace(pane)
	if trimmed == "" {
		return ClaudeDead
	}

	// Look at the tail of the buffer — that's where Claude renders state.
	tail := tailLines(pane, 60)

	if reApiError.MatchString(tail) {
		return ClaudeError
	}
	if reEscToInterrupt.MatchString(tail) {
		return ClaudeRunning
	}

	// "Needs Input" trumps "Idle" because the prompt box also appears around
	// approval / selector dialogs.
	if isApprovalPrompt(tail) || isSelectorPrompt(tail) || isQuestionPrompt(tail) {
		return ClaudeNeedsInput
	}

	if reClaudePrompt.MatchString(tail) {
		return ClaudeIdle
	}

	return ClaudeDead
}

func isApprovalPrompt(s string) bool {
	matches := reNumberedOption.FindAllString(s, -1)
	return len(matches) >= 2
}

func isSelectorPrompt(s string) bool {
	return reSelectorArrow.MatchString(s)
}

func isQuestionPrompt(s string) bool {
	return reQuestionMark.MatchString(s) && reClaudePrompt.MatchString(s)
}

func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
