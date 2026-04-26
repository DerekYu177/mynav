package app

import (
	"fmt"
	"sort"
	"time"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
	"github.com/awesome-gocui/gocui"
	"github.com/gookit/color"
)

// Sessions view displaying active workspace sessions.
type Sessions struct {
	view  *tui.View
	table *tui.TableRenderer[*core.Session]

	// loading flag to display loading (not atomic as it should only be touched in the mainloop)
	loading bool

	// to kill the status-refresh routine
	done chan bool
}

func newSessionsView() *Sessions {
	s := &Sessions{
		done: make(chan bool),
	}
	return s
}

func (s *Sessions) selected() *core.Session {
	_, session := s.table.SelectedRow()
	if session != nil {
		return session.Value
	}
	return nil
}

func (s *Sessions) selectSession(session *core.Session) {
	s.table.SelectRowByValue(func(session2 *core.Session) bool {
		return session2.Name == session.Name
	})
}

func (s *Sessions) getLoading() bool {
	return s.loading
}

func (s *Sessions) setLoading(b bool) {
	s.loading = b
}

func (s *Sessions) refreshPreview() {
	session := s.selected()
	if session == nil {
		a.preview.setSession(nil)
		return
	}

	a.preview.setSession(session)
}

func (s *Sessions) focus() {
	a.focusView(s.view)
	s.refreshDown()
}

func (s *Sessions) refreshDown() {
	a.comment.show(s.selected())
	a.worker.Queue(func() {
		s.refreshPreview()
		a.ui.Update(func() {
			a.preview.render()
			a.comment.render()
		})
	})
}

func (s *Sessions) refresh() {
	sessions := a.api.AllSessions()
	// sort by last attached
	sort.Slice(sessions, func(i, j int) bool {
		t1 := core.UnixTime(sessions[i].LastAttached)
		t2 := core.UnixTime(sessions[j].LastAttached)
		return t1.After(t2)
	})

	// fill table
	tableRows := make([]*tui.TableRow[*core.Session], 0)
	for _, s := range sessions {
		timeStr := core.TimeAgo(core.UnixTime(s.LastAttached))
		status := a.api.ClaudeStatus(s)
		tableRows = append(tableRows, &tui.TableRow[*core.Session]{
			Cols: []string{
				s.DisplayName(),
				status.String(),
				timeStr,
			},
			Value: s,
		})
	}
	s.table.Fill(tableRows)
}

func (s *Sessions) render() {
	s.view.Clear()
	a.ui.Resize(s.view, getViewPosition(s.view.Name()))

	// update page row marker
	row, _ := s.table.SelectedRow()
	size := s.table.Size()
	s.view.Subtitle = fmt.Sprintf(" %d / %d ", min(row+1, size), size)

	if s.getLoading() {
		fmt.Fprintln(s.view, "Loading...")
		return
	}

	// renders table and updates only the last-attached column. Claude status
	// is captured by refresh() (off the render path) because it shells out to
	// tmux capture-pane and would be too expensive on every frame.
	isFocused := a.ui.IsFocused(s.view)
	s.table.RenderTable(s.view, func(i int, tr *tui.TableRow[*core.Session]) bool {
		return isFocused
	}, func(i int, tr *tui.TableRow[*core.Session]) {
		tr.Cols[len(tr.Cols)-1] = core.TimeAgo(core.UnixTime(tr.Value.LastAttached))
	})
}

func (s *Sessions) attach(session *core.Session) {
	if core.IsTmuxSession() {
		toast("A tmux session is already active", toastWarn)
		return
	}

	start := time.Now()
	err := a.runAction(func() error {
		return session.Attach()
	})

	if err != nil {
		toast(err.Error(), toastError)
	} else {
		timeTaken := time.Since(start)
		s := fmt.Sprintf("Detached session %s - %s active", session.DisplayName(), core.TimeDeltaStr(timeTaken))
		toast(s, toastInfo)
	}

	a.refresh(session)
}

func (s *Sessions) init() {
	s.view = a.ui.SetView(getViewPosition(SessionsView))
	s.view.Title = " Sessions "
	a.styleView(s.view)

	sizeX, sizeY := s.view.Size()
	titles := []string{
		"Name",
		"Status",
		"Last Attached",
	}
	proportions := []float64{
		0.45,
		0.30,
		0.25,
	}
	styles := []color.Style{
		workspaceNameColor,
		sessionMarkerColor,
		timestampColor,
	}
	s.table = tui.NewTableRenderer[*core.Session]()
	s.table.Init(sizeX, sizeY, titles, proportions)
	s.table.SetStyles(styles)

	down := func() {
		s.table.Down()
		s.refreshDown()
	}
	up := func() {
		s.table.Up()
		s.refreshDown()
	}
	a.ui.KeyBinding(s.view).
		Set('j', "Move down", down).
		Set('k', "Move up", up).
		Set(gocui.KeyArrowDown, "Move down", down).
		Set(gocui.KeyArrowUp, "Move up", up).
		Set('g', "Go to top", func() {
			s.table.Top()
			s.refreshDown()
		}).
		Set('G', "Go to bottom", func() {
			s.table.Bottom()
			s.refreshDown()
		}).
		Set(gocui.KeyEnter, "Open Session", func() {
			session := s.selected()
			if session == nil {
				return
			}

			s.attach(session)
		}).
		Set('D', "Kill session", func() {
			session := s.selected()
			if session == nil {
				return
			}
			alert(func(b bool) {
				if !b {
					return
				}

				if err := session.Kill(); err != nil {
					toast(err.Error(), toastError)
					return
				}

				a.refresh(session)
				toast("Killed session "+session.DisplayName(), toastInfo)
			}, fmt.Sprintf("Are you sure you want to delete session for %s?", session.DisplayName()))
		}).
		Set('a', "Create a Sesssion", func() {
			editor(func(name string) {
				session, err := a.api.NewSession(name)
				if err != nil {
					toast(err.Error(), toastError)
					return
				}
				s.attach(session)
			}, func() {}, "Session Name", smallEditorSize, "")
		}).
		Set('c', "Approve Claude prompt", func() {
			session := s.selected()
			if session == nil {
				return
			}
			if a.api.ClaudeStatus(session) != core.ClaudeNeedsInput {
				toast("Claude is not waiting for input", toastWarn)
				return
			}
			approvalOverlay(session)
		}).
		Set('n', "Edit session note", func() {
			session := s.selected()
			if session == nil {
				return
			}
			current := a.api.SessionComment(session)
			editor(func(text string) {
				a.api.SetSessionComment(session, text)
				a.comment.show(session)
				a.comment.render()
			}, func() {}, "Note", largeEditorSize, current)
		}).
		Set('?', "Toggle cheatsheet", func() {
			help(s.view)
		})

	// periodically re-capture pane content and refresh claude status. Runs
	// off the UI thread; refresh() will reorder by LastAttached so we
	// preserve the user's selection across re-fills.
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-t.C:
				if a.attached.Load() {
					continue
				}
				selected := s.selected()
				s.refresh()
				if selected != nil {
					s.selectSession(selected)
				}
				a.ui.Update(func() {
					s.render()
				})
			}
		}
	}()
}
