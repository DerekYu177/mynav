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

func getViewPosition(viewName string) *tui.ViewPosition {
	maxX, maxY := a.ui.Size()
	positionMap := map[string]*tui.ViewPosition{}

	// sessions: full left column
	positionMap[SessionsView] = tui.NewViewPosition(
		SessionsView,
		0, 0,
		maxX/3-1, maxY-1,
		0,
	)

	// preview: full right side
	positionMap[PreviewView] = tui.NewViewPosition(
		PreviewView,
		maxX/3, 0,
		maxX-1, maxY-1,
		0,
	)

	return positionMap[viewName]
}
