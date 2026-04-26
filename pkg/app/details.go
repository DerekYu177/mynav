package app

import (
	"fmt"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
)

// Details renders a small overlay on the top-right of the preview area
// showing the selected session's last-attached time and note. It is
// hidden (removed from gocui) when there's no session selected.
type Details struct {
	view    *tui.View
	session *core.Session

	// visible tracks whether gocui currently knows about the view, so
	// render() only adds / removes on transitions instead of every frame.
	visible bool
}

func newDetails() *Details { return &Details{} }

// show updates which session this overlay describes. Pass nil to hide.
func (d *Details) show(s *core.Session) { d.session = s }

func (d *Details) render() {
	want := d.session != nil

	switch {
	case want && !d.visible:
		d.view = a.ui.SetView(getViewPosition(DetailsView))
		d.view.Title = " Details "
		a.styleView(d.view)
		d.visible = true
	case !want && d.visible:
		a.ui.DeleteView(d.view)
		d.visible = false
		return
	case !want:
		return
	}

	a.ui.Resize(d.view, getViewPosition(DetailsView))
	d.view.Clear()

	inner := detailsWidth - 2

	ts := core.TimeAgo(core.UnixTime(d.session.LastAttached))
	fmt.Fprintln(d.view, " "+timestampColor.Sprint(truncateRunes(ts, inner-2)))

	note := a.api.SessionComment(d.session)
	var noteLine string
	if note == "" {
		noteLine = timestampColor.Sprint("(no note)")
	} else {
		noteLine = workspaceNameColor.Sprint(truncateRunes(note, inner-2))
	}
	fmt.Fprintln(d.view, " "+noteLine)
}
