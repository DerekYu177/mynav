package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
)

// Details renders two floating overlays above the preview describing the
// currently selected session: a SessionInfo pane (top-center) with the
// session name, backing worktree (when managed), and time-since-attached;
// and a Comment pane (top-right) with the session note word-wrapped to
// fit. Both panes are removed from gocui when no session is selected.
type Details struct {
	infoView    *tui.View
	commentView *tui.View
	session     *core.Session

	// visibility flags so render() only adds / removes views on
	// transitions instead of every frame.
	infoVisible    bool
	commentVisible bool

	// infoLines / commentLines are the content-line counts produced by
	// the most recent render. getViewPosition reads them to compute
	// each pane's bottom edge — the Comment pane in particular grows
	// with wrapped-line count, capped at commentMaxHeight.
	infoLines    int
	commentLines int
}

func newDetails() *Details { return &Details{} }

// show updates which session these overlays describe. Pass nil to hide.
func (d *Details) show(s *core.Session) { d.session = s }

func (d *Details) render() {
	if d.session == nil {
		d.hideInfo()
		d.hideComment()
		return
	}

	// Comment is rendered first so commentVisible is up-to-date by the
	// time the SessionInfo pane checks for collisions.
	d.renderComment()
	d.renderInfo()
}

func (d *Details) hideInfo() {
	if !d.infoVisible {
		return
	}
	a.ui.DeleteView(d.infoView)
	d.infoView = nil
	d.infoVisible = false
	d.infoLines = 0
}

func (d *Details) hideComment() {
	if !d.commentVisible {
		return
	}
	a.ui.DeleteView(d.commentView)
	d.commentView = nil
	d.commentVisible = false
	d.commentLines = 0
}

// renderInfo draws the SessionInfo pane: two lines, both centered.
// Line 1 is the session name plus the backing worktree name when the
// session is worktree-managed (`<name> | <worktree>`); for unmanaged
// sessions it's just the session name. Line 2 is the time-since-attached.
func (d *Details) renderInfo() {
	worktree := a.api.SessionWorktreePath(d.session)
	worktreeName := ""
	if worktree != "" {
		worktreeName = filepath.Base(worktree)
		// World layout is `<name>/src/.git`, so the marker path ends in
		// `/src` for World worktrees. Strip it so the pane shows the
		// worktree name the user thinks of, not the inner src dir.
		if worktreeName == "src" {
			worktreeName = filepath.Base(filepath.Dir(worktree))
		}
	}

	d.infoLines = 2

	if !d.infoVisible {
		d.infoView = a.ui.SetView(getViewPosition(SessionInfoView))
		d.infoView.Title = " Session "
		a.styleView(d.infoView)
		d.infoVisible = true
	}
	a.ui.Resize(d.infoView, getViewPosition(SessionInfoView))
	d.infoView.Clear()

	inner := sessionInfoPaneWidth - 2

	var nameLine string
	if worktreeName != "" {
		const sep = " | "
		nameBudget := (inner - utf8.RuneCountInString(sep)) / 2
		name := truncateRunes(d.session.Name, nameBudget)
		wt := truncateRunes(worktreeName, nameBudget)
		nameLine = workspaceNameColor.Sprint(name) + timestampColor.Sprint(sep) + workspaceNameColor.Sprint(wt)
	} else {
		nameLine = workspaceNameColor.Sprint(truncateRunes(d.session.Name, inner))
	}
	fmt.Fprintln(d.infoView, centerLine(nameLine, inner))

	timeLine := core.TimeAgo(core.UnixTime(d.session.LastAttached))
	fmt.Fprintln(d.infoView, centerLine(timestampColor.Sprint(truncateRunes(timeLine, inner)), inner))
}

// renderComment draws the Comment pane when the selected session has a
// non-empty note. Hidden entirely when there is no comment so empty
// boxes don't clutter unannotated sessions. Long comments word-wrap;
// overflow past commentMaxHeight gets a trailing ellipsis on the last
// visible line.
func (d *Details) renderComment() {
	note := a.api.SessionComment(d.session)
	if note == "" {
		d.hideComment()
		return
	}

	inner := commentPaneWidth - 2
	contentWidth := inner - 2 // 1-col padding each side for visual breathing room
	lines := wrapText(note, contentWidth)

	maxLines := commentMaxHeight - 2
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		if maxLines > 0 {
			last := lines[maxLines-1]
			lines[maxLines-1] = truncateRunes(last, contentWidth-1) + "…"
		}
	}
	d.commentLines = len(lines)
	if d.commentLines == 0 {
		d.hideComment()
		return
	}

	if !d.commentVisible {
		d.commentView = a.ui.SetView(getViewPosition(CommentView))
		d.commentView.Title = " Comment "
		a.styleView(d.commentView)
		d.commentVisible = true
	}
	a.ui.Resize(d.commentView, getViewPosition(CommentView))
	d.commentView.Clear()

	for _, line := range lines {
		fmt.Fprintln(d.commentView, " "+line)
	}
}

// wrapText breaks s into lines no wider than width runes, splitting at
// word boundaries. Words that on their own exceed width are hard-split
// into width-sized chunks so a long URL or path can't blow the layout.
// Returns nil for empty input or non-positive width.
func wrapText(s string, width int) []string {
	if width <= 0 || strings.TrimSpace(s) == "" {
		return nil
	}
	words := strings.Fields(s)
	var lines []string
	var cur string
	for _, w := range words {
		// hard-split words longer than the line width
		for utf8.RuneCountInString(w) > width {
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			r := []rune(w)
			lines = append(lines, string(r[:width]))
			w = string(r[width:])
		}
		switch {
		case cur == "":
			cur = w
		case utf8.RuneCountInString(cur)+1+utf8.RuneCountInString(w) <= width:
			cur += " " + w
		default:
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// centerLine pads s with leading spaces so its visible content sits
// horizontally centered in width runes. Strings already at or beyond
// the width are returned as-is. The width argument counts visible
// runes — color escape sequences in s are passed through untouched.
func centerLine(s string, width int) string {
	visible := visibleRuneCount(s)
	if visible >= width {
		return s
	}
	pad := (width - visible) / 2
	return strings.Repeat(" ", pad) + s
}

// visibleRuneCount counts runes in s while skipping ANSI CSI escape
// sequences (ESC [ ... letter), so styled strings produced by gookit
// fmt are measured by their on-screen width, not their byte length.
func visibleRuneCount(s string) int {
	n := 0
	inEscape := false
	for _, r := range s {
		switch {
		case inEscape:
			// CSI ends on the first letter
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
		case r == 0x1b:
			inEscape = true
		default:
			n++
		}
	}
	return n
}
