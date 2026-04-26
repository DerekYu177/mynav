package app

import (
	"fmt"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/awesome-gocui/gocui"
	"github.com/gookit/color"
)

// ApprovalDialog is the view name for the Claude approval overlay.
const ApprovalDialog = "ApprovalDialog"

// approvalOverlay opens a modal that lets the user respond to a Claude
// approval prompt without leaving mynav. It targets the active pane of the
// passed session and dispatches keystrokes via tmux send-keys.
func approvalOverlay(session *core.Session) {
	if session == nil {
		return
	}

	pane := session.ActivePaneCapture()
	mode := core.DetectApprovalMode(pane)
	target := session.ActivePaneID()

	if target == "" {
		toast("Could not resolve active pane id", toastError)
		return
	}

	switch mode {
	case core.ApprovalOneKey:
		openOneKeyOverlay(session, target)
	case core.ApprovalSelector:
		openSelectorOverlay(session, target)
	case core.ApprovalNone:
		toast("No approval prompt detected — press 'e' for text input instead", toastWarn)
	}
}

func openOneKeyOverlay(session *core.Session, target string) {
	v := a.ui.SetCenteredView(ApprovalDialog, 50, 6, 0, 0)
	v.Title = " Approve "
	v.FrameColor = gocui.ColorWhite
	a.styleView(v)

	prevView := a.ui.FocusedView()
	closeAndFocus := func() {
		a.ui.DeleteView(v)
		if prevView != nil {
			a.ui.FocusView(prevView)
		}
	}

	send := func(digit string) {
		if err := core.SendKeys(target, digit); err != nil {
			toast(fmt.Sprintf("send-keys failed: %s", err), toastError)
			return
		}
		closeAndFocus()
		toast(fmt.Sprintf("Sent %s to %s", digit, session.DisplayName()), toastInfo)
	}

	a.ui.KeyBinding(v).
		Set('1', "Yes", func() { send("1") }).
		Set('2', "Yes (always)", func() { send("2") }).
		Set('3', "No", func() { send("3") }).
		Set(gocui.KeyEsc, "Cancel", closeAndFocus).
		Set('q', "Cancel", closeAndFocus)

	a.ui.FocusView(v)

	v.Clear()
	fmt.Fprintln(v, color.Note.Sprint(" Claude is waiting for approval"))
	fmt.Fprintln(v)
	line := fmt.Sprintf(" %s   %s   %s   %s ",
		approvalKey("1", "Yes"),
		approvalKey("2", "Always"),
		approvalKey("3", "No"),
		color.Danger.Sprint("Esc")+timestampColor.Sprint(" cancel"),
	)
	fmt.Fprintln(v, line)
}

func openSelectorOverlay(session *core.Session, target string) {
	v := a.ui.SetCenteredView(ApprovalDialog, 50, 6, 0, 0)
	v.Title = " Select "
	v.FrameColor = gocui.ColorWhite
	a.styleView(v)

	prevView := a.ui.FocusedView()
	closeAndFocus := func() {
		a.ui.DeleteView(v)
		if prevView != nil {
			a.ui.FocusView(prevView)
		}
	}

	move := func(direction string) {
		if err := core.SendKeys(target, direction); err != nil {
			toast(fmt.Sprintf("send-keys failed: %s", err), toastError)
		}
	}
	confirm := func() {
		if err := core.SendKeys(target, "Enter"); err != nil {
			toast(fmt.Sprintf("send-keys failed: %s", err), toastError)
			return
		}
		closeAndFocus()
		toast(fmt.Sprintf("Confirmed in %s", session.DisplayName()), toastInfo)
	}

	a.ui.KeyBinding(v).
		Set('k', "Up", func() { move("Up") }).
		Set('j', "Down", func() { move("Down") }).
		Set(gocui.KeyArrowUp, "Up", func() { move("Up") }).
		Set(gocui.KeyArrowDown, "Down", func() { move("Down") }).
		Set(gocui.KeyEnter, "Confirm", confirm).
		Set(gocui.KeyEsc, "Cancel", closeAndFocus).
		Set('q', "Cancel", closeAndFocus)

	a.ui.FocusView(v)

	v.Clear()
	fmt.Fprintln(v, color.Note.Sprint(" Claude is showing a selector"))
	fmt.Fprintln(v)
	line := fmt.Sprintf(" %s %s   %s   %s ",
		sessionMarkerColor.Sprint("↑↓"),
		timestampColor.Sprint("move"),
		sessionMarkerColor.Sprint("Enter")+timestampColor.Sprint(" confirm"),
		color.Danger.Sprint("Esc")+timestampColor.Sprint(" cancel"),
	)
	fmt.Fprintln(v, line)
}

func approvalKey(key, label string) string {
	return sessionMarkerColor.Sprint(key) + timestampColor.Sprint(" "+label)
}

