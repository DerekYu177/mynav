package app

import (
	"github.com/GianlucaP106/mynav/pkg/tui"
)

// Views.
const (
	SessionsView = "SessionsView"
	PreviewView  = "TmuxPreviewView"
)

// Dialogs.
const (
	EditorDialog           = "EditorDialog"
	ConfirmationDialog     = "ConfirmationDialog"
	ToastDialog            = "ToastDialogView"
	HelpDialog             = "HelpDialog"
	SearchListDialog1View  = "SearchListDialog1"
	SearchListDialog2View  = "SearchListDialog2"
	SearchListDialog3View  = "SearchListDialog3"
	SearchListDialogBgView = "SearchListDialogBg"
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

	return positionMap[viewName]
}
