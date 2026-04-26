package app

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
	"github.com/awesome-gocui/gocui"
	"github.com/gookit/color"
)

// Sessions view rendering active workspace sessions as a 2-D grid of cells.
type Sessions struct {
	view *tui.View

	// cells, ordered for stable position (oldest → newest by tmux session_created)
	cells []*core.Session

	// claude status cached in lockstep with cells; populated by refresh().
	// Computing status shells out to `tmux capture-pane`, so we keep it off
	// the render path — the 3 s ticker is the only place it runs.
	statuses []core.ClaudeStatus

	// currently highlighted cell
	selIdx int

	// columns the most recent render used (set by render(), read by hjkl)
	cols int

	// loading flag (only touched on the mainloop)
	loading bool

	// to kill the status-refresh routine
	done chan bool
}

// Cell layout constants. Each cell is a 20-wide / 3-tall box drawn from
// box-drawing runes — top edge, one content line ("name + status dot"),
// and bottom edge — separated by a single blank column. The note and
// last-attached time live in the details overlay, not the cell itself.
const (
	cellWidth      = 20
	cellHeight     = 3
	cellNameLength = 14
	cellGutterX    = 1
	cellGutterY    = 0
)

var (
	cellBorderColor   = color.New(color.FgDarkGray)
	cellSelectedColor = color.New(color.FgGreen, color.Bold)

	statusRunningColor    = color.New(color.FgGreen, color.Bold)
	statusNeedsInputColor = color.New(color.FgYellow, color.Bold)
	statusIdleColor       = color.New(color.FgWhite)
	statusErrorColor      = color.New(color.FgRed, color.Bold)
	statusDeadColor       = color.New(color.FgDarkGray)
)

func newSessionsView() *Sessions {
	return &Sessions{
		done: make(chan bool),
	}
}

func (s *Sessions) selected() *core.Session {
	if s.selIdx < 0 || s.selIdx >= len(s.cells) {
		return nil
	}
	return s.cells[s.selIdx]
}

func (s *Sessions) selectSession(target *core.Session) {
	if target == nil {
		return
	}
	for i, c := range s.cells {
		if c.Name == target.Name {
			s.selIdx = i
			return
		}
	}
}

func (s *Sessions) getLoading() bool { return s.loading }
func (s *Sessions) setLoading(b bool) { s.loading = b }

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
	a.details.show(s.selected())
	a.worker.Queue(func() {
		s.refreshPreview()
		a.ui.Update(func() {
			a.preview.render()
		})
	})
}

func (s *Sessions) refresh() {
	sessions := a.api.AllSessions()
	// Sort by created time (oldest first) so cells keep stable positions
	// across refreshes — new sessions appear at the end without shuffling
	// the existing layout.
	sort.Slice(sessions, func(i, j int) bool {
		t1 := core.UnixTime(sessions[i].Created)
		t2 := core.UnixTime(sessions[j].Created)
		return t1.Before(t2)
	})
	s.cells = sessions
	s.statuses = make([]core.ClaudeStatus, len(sessions))
	for i, sess := range sessions {
		s.statuses[i] = a.api.ClaudeStatus(sess)
	}
	if s.selIdx >= len(s.cells) {
		s.selIdx = max(0, len(s.cells)-1)
	}
	if s.selIdx < 0 {
		s.selIdx = 0
	}
}

func (s *Sessions) render() {
	s.view.Clear()
	a.ui.Resize(s.view, getViewPosition(s.view.Name()))

	sx, _ := s.view.Size()
	cols := (sx + cellGutterX) / (cellWidth + cellGutterX)
	if cols < 1 {
		cols = 1
	}
	s.cols = cols

	s.view.Subtitle = fmt.Sprintf(" %d sessions ", len(s.cells))

	if s.getLoading() {
		fmt.Fprintln(s.view, "Loading...")
		return
	}

	if len(s.cells) == 0 {
		fmt.Fprintln(s.view, " No sessions yet — press 'a' to create one")
		return
	}

	isFocused := a.ui.IsFocused(s.view)

	gutter := strings.Repeat(" ", cellGutterX)
	for start := 0; start < len(s.cells); start += cols {
		end := start + cols
		if end > len(s.cells) {
			end = len(s.cells)
		}

		// Render every cell in this row of cells, then interleave by line.
		row := make([][]string, 0, end-start)
		for i := start; i < end; i++ {
			row = append(row, s.renderCell(s.cells[i], s.statusAt(i), isFocused && i == s.selIdx))
		}

		for line := 0; line < cellHeight; line++ {
			parts := make([]string, len(row))
			for ci, cell := range row {
				parts[ci] = cell[line]
			}
			fmt.Fprintln(s.view, strings.Join(parts, gutter))
		}

		for g := 0; g < cellGutterY && end < len(s.cells); g++ {
			fmt.Fprintln(s.view)
		}
	}
}

// statusAt returns the cached status at index i, defaulting to ClaudeDead
// when the parallel slice is missing (e.g. between refreshes).
func (s *Sessions) statusAt(i int) core.ClaudeStatus {
	if i < 0 || i >= len(s.statuses) {
		return core.ClaudeDead
	}
	return s.statuses[i]
}

// renderCell returns the cellHeight strings that visually compose one cell.
func (s *Sessions) renderCell(sess *core.Session, status core.ClaudeStatus, selected bool) []string {
	border := cellBorderColor
	if selected {
		border = cellSelectedColor
	}

	// Inner content width — the single content line must visually occupy
	// this many runes. The vertical edges are added afterwards.
	inner := cellWidth - 2

	icon := statusIcon(status)
	name := truncateRunes(sess.DisplayName(), cellNameLength)
	nameStyled := workspaceNameColor.Sprint(padRightRunes(name, cellNameLength))

	// " name(14) icon " — 1 + 14 + 1 + 1 + 1 = 18 ✓ (inner)
	content := " " + nameStyled + " " + icon + " "

	top := border.Sprint("┌" + strings.Repeat("─", inner) + "┐")
	bot := border.Sprint("└" + strings.Repeat("─", inner) + "┘")
	edge := border.Sprint("│")

	return []string{
		top,
		edge + content + edge,
		bot,
	}
}

func statusIcon(s core.ClaudeStatus) string {
	switch s {
	case core.ClaudeRunning:
		return statusRunningColor.Sprint("●")
	case core.ClaudeNeedsInput:
		return statusNeedsInputColor.Sprint("●")
	case core.ClaudeIdle:
		return statusIdleColor.Sprint("○")
	case core.ClaudeError:
		return statusErrorColor.Sprint("●")
	case core.ClaudeDead:
		fallthrough
	default:
		return statusDeadColor.Sprint("·")
	}
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func padRightRunes(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(r))
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
		msg := fmt.Sprintf("Detached session %s - %s active", session.DisplayName(), core.TimeDeltaStr(timeTaken))
		toast(msg, toastInfo)
	}

	a.refresh(session)
}

func (s *Sessions) init() {
	s.view = a.ui.SetView(getViewPosition(SessionsView))
	s.view.Title = " Sessions "
	a.styleView(s.view)

	// Navigation wraps within the current row (h/l) and within the
	// current column (j/k). A column may not have a cell in every row
	// (the trailing row can be partial), so vertical wrap snaps to the
	// last row that actually has the target column.
	left := func() {
		if len(s.cells) == 0 || s.cols == 0 {
			return
		}
		row := s.selIdx / s.cols
		col := s.selIdx % s.cols
		if col == 0 {
			end := (row+1)*s.cols - 1
			if end >= len(s.cells) {
				end = len(s.cells) - 1
			}
			s.selIdx = end
		} else {
			s.selIdx--
		}
		s.refreshDown()
	}
	right := func() {
		if len(s.cells) == 0 || s.cols == 0 {
			return
		}
		row := s.selIdx / s.cols
		atRowEnd := (s.selIdx+1)%s.cols == 0 || s.selIdx == len(s.cells)-1
		if atRowEnd {
			s.selIdx = row * s.cols
		} else {
			s.selIdx++
		}
		s.refreshDown()
	}
	down := func() {
		if len(s.cells) == 0 || s.cols == 0 {
			return
		}
		next := s.selIdx + s.cols
		if next >= len(s.cells) {
			s.selIdx = s.selIdx % s.cols
		} else {
			s.selIdx = next
		}
		s.refreshDown()
	}
	up := func() {
		if len(s.cells) == 0 || s.cols == 0 {
			return
		}
		prev := s.selIdx - s.cols
		if prev < 0 {
			col := s.selIdx % s.cols
			rows := (len(s.cells) + s.cols - 1) / s.cols
			bottom := (rows-1)*s.cols + col
			if bottom >= len(s.cells) {
				bottom -= s.cols
			}
			s.selIdx = bottom
		} else {
			s.selIdx = prev
		}
		s.refreshDown()
	}

	a.ui.KeyBinding(s.view).
		Set('h', "Move left", left).
		Set('l', "Move right", right).
		Set('j', "Move down", down).
		Set('k', "Move up", up).
		Set('g', "Go to first", func() {
			s.selIdx = 0
			s.refreshDown()
		}).
		Set('G', "Go to last", func() {
			if len(s.cells) > 0 {
				s.selIdx = len(s.cells) - 1
			}
			s.refreshDown()
		}).
		Set(gocui.KeyEnter, "Open session", func() {
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
		Set('a', "Create a session", func() {
			editor(func(name string) {
				session, err := a.api.NewSession(name)
				if err != nil {
					toast(err.Error(), toastError)
					return
				}
				s.attach(session)
			}, func() {}, "Session Name", smallEditorSize, "")
		}).
		Set('e', "Enter pane (zoomed)", func() {
			session := s.selected()
			if session == nil {
				return
			}
			if core.IsTmuxSession() {
				toast("A tmux session is already active", toastWarn)
				return
			}
			err := a.runAction(func() error {
				return a.api.AttachZoomed(session)
			})
			if err != nil {
				toast(err.Error(), toastError)
			}
			a.refresh(session)
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
				s.render()
			}, func() {}, "Note", largeEditorSize, current)
		}).
		Set('?', "Toggle cheatsheet", func() {
			help(s.view)
		})

	// Periodic status refresh. The ticker reorders cells by created time
	// so we re-pin the selected session by name each tick — the user's
	// cursor doesn't drift when statuses or attach times change.
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
