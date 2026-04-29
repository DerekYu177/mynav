package app

import (
	"github.com/GianlucaP106/mynav/pkg/tui"
)

// Views.
const (
	SessionsView    = "SessionsView"
	PreviewView     = "TmuxPreviewView"
	SessionInfoView = "SessionInfoView"
	CommentView     = "CommentView"
)

// Floating-pane sizing for the SessionInfo + Comment overlays. Both panes
// sit above the preview frame (anchored to row 1). SessionInfo is centered
// horizontally; Comment is anchored to the preview's right edge. Comment's
// height auto-grows with the wrapped comment, capped at commentMaxHeight.
const (
	sessionInfoPaneWidth = 52
	commentPaneWidth     = 36
	paneRightMargin      = 2
	paneGutter           = 2
	paneTopRow           = 1
	sessionInfoHeight    = 4
	commentMaxHeight     = 12
)

// Dialogs.
const (
	EditorDialog       = "EditorDialog"
	ConfirmationDialog = "ConfirmationDialog"
	ToastDialog        = "ToastDialogView"
	HelpDialog         = "HelpDialog"
)

// computeSessionsHeight returns the number of terminal rows the sessions
// strip should occupy. The strip is exactly tall enough to hold the cell
// rows produced by the current session count and the current terminal
// width — a single-row fallback covers init / empty states where the
// real cell list isn't populated yet.
func computeSessionsHeight() int {
	const oneRow = cellHeight + 2 // 1 row of cells + outer frame

	if a == nil || a.sessions == nil || len(a.sessions.cells) == 0 {
		return oneRow
	}

	sx, _ := a.ui.Size()
	cols := (sx + cellGutterX) / (cellWidth + cellGutterX)
	if cols < 1 {
		cols = 1
	}
	rows := (len(a.sessions.cells) + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	return rows*cellHeight + (rows-1)*cellGutterY + 2
}

// commentPaneHeight returns the desired height of the Comment pane based
// on the wrapped-line count produced by the most recent details render.
// Falls back to a single-line shape when nothing has been computed yet.
func commentPaneHeight() int {
	if a == nil || a.details == nil {
		return 3
	}
	lines := a.details.commentLines
	if lines < 1 {
		lines = 1
	}
	if lines > commentMaxHeight-2 {
		lines = commentMaxHeight - 2
	}
	return lines + 2
}

// sessionInfoPaneHeight returns the desired height of the SessionInfo
// pane. Managed sessions show 3 content lines (name, worktree, time);
// unmanaged sessions drop the worktree line for 2 content lines, so the
// box stays compact. The fallback covers init before any session is
// selected.
func sessionInfoPaneHeight() int {
	if a == nil || a.details == nil {
		return sessionInfoHeight
	}
	lines := a.details.infoLines
	if lines < 1 {
		lines = 1
	}
	return lines + 2
}

func getViewPosition(viewName string) *tui.ViewPosition {
	maxX, maxY := a.ui.Size()
	sh := computeSessionsHeight()
	positionMap := map[string]*tui.ViewPosition{}

	// preview: top, fills everything above the sessions grid
	positionMap[PreviewView] = tui.NewViewPosition(
		PreviewView,
		0, 0,
		maxX-1, maxY-sh-1,
		0,
	)

	// sessions: bottom strip, full width
	positionMap[SessionsView] = tui.NewViewPosition(
		SessionsView,
		0, maxY-sh,
		maxX-1, maxY-1,
		0,
	)

	// Comment pane: anchored to the preview's right edge with a small
	// gutter so its corners don't merge with the preview frame. Height
	// grows with the wrapped comment.
	commentX1 := maxX - 1 - paneRightMargin
	commentX0 := commentX1 - commentPaneWidth + 1
	commentY0 := paneTopRow
	commentY1 := commentY0 + commentPaneHeight() - 1
	positionMap[CommentView] = tui.NewViewPosition(
		CommentView,
		commentX0, commentY0,
		commentX1, commentY1,
		0,
	)

	// SessionInfo pane: horizontally centered over the preview. If the
	// terminal is too narrow for both panes side-by-side, the comment
	// pane wins the right slot and SessionInfo shifts left until it
	// stops just shy of the comment pane (keeping a 2-col gutter).
	infoX0 := (maxX - sessionInfoPaneWidth) / 2
	if infoX0 < 0 {
		infoX0 = 0
	}
	infoX1 := infoX0 + sessionInfoPaneWidth - 1
	if a != nil && a.details != nil && a.details.commentVisible {
		if infoX1 >= commentX0-paneGutter {
			infoX1 = commentX0 - paneGutter - 1
			infoX0 = infoX1 - sessionInfoPaneWidth + 1
			if infoX0 < 0 {
				infoX0 = 0
			}
		}
	}
	infoY0 := paneTopRow
	infoY1 := infoY0 + sessionInfoPaneHeight() - 1
	positionMap[SessionInfoView] = tui.NewViewPosition(
		SessionInfoView,
		infoX0, infoY0,
		infoX1, infoY1,
		0,
	)

	return positionMap[viewName]
}
