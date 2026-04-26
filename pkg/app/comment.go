package app

import (
	"fmt"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
)

// Comment renders the saved comment for the currently selected session.
type Comment struct {
	view    *tui.View
	session *core.Session
}

func newComment() *Comment {
	return &Comment{}
}

func (c *Comment) init() {
	c.view = a.ui.SetView(getViewPosition(CommentView))
	c.view.Title = " Comment "
	c.view.Wrap = true
	a.styleView(c.view)
}

// show updates which session this view is reading the comment for.
func (c *Comment) show(s *core.Session) {
	c.session = s
}

func (c *Comment) render() {
	c.view.Clear()
	a.ui.Resize(c.view, getViewPosition(c.view.Name()))

	if c.session == nil {
		return
	}

	text := a.api.SessionComment(c.session)
	if text == "" {
		fmt.Fprint(c.view, timestampColor.Sprint("No comment — press n to add one"))
		return
	}
	fmt.Fprintln(c.view, text)
}
