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

// sessionsHeight is the number of rows reserved for the sessions grid at
// the bottom of the screen. Two rows of cells plus a row for borders / a
// little breathing room covers the realistic session count without ever
// needing to scroll.
const sessionsHeight = 11

func getViewPosition(viewName string) *tui.ViewPosition {
	maxX, maxY := a.ui.Size()
	positionMap := map[string]*tui.ViewPosition{}

	// preview: top, fills everything above the sessions grid
	positionMap[PreviewView] = tui.NewViewPosition(
		PreviewView,
		0, 0,
		maxX-1, maxY-sessionsHeight-1,
		0,
	)

	// sessions: bottom strip, full width
	positionMap[SessionsView] = tui.NewViewPosition(
		SessionsView,
		0, maxY-sessionsHeight,
		maxX-1, maxY-1,
		0,
	)

	return positionMap[viewName]
}
