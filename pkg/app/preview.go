package app

import (
	"fmt"
	"sync"
	"time"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
)

type Preview struct {
	view *tui.View

	// preview raw content
	previews   []string
	previewIdx int
	previewMu  sync.RWMutex

	// session to show
	// this needs to be kept track of because of periodic refresh
	session   *core.Session
	sessionMu sync.RWMutex

	// to kill the refresh routine
	done chan bool
}

func newPreview() *Preview {
	p := &Preview{}
	return p
}

func (p *Preview) init(v *tui.View) {
	p.done = make(chan bool)
	p.view = v
	p.view.Title = " Preview "
	a.styleView(p.view)
	p.setPreviews(nil)
	go func() {
		t := time.NewTicker(time.Second * 3)
		for {
			select {
			case <-p.done:
				return
			case <-t.C:
				if !a.attached.Load() {
					p.refresh()
					a.ui.Update(func() {
						p.render()
					})
				}

			}
		}
	}()
}

func (p *Preview) setSession(session *core.Session) {
	if session == nil {
		p.sessionMu.Lock()
		p.session = nil
		p.sessionMu.Unlock()

		p.setPreviews(nil)
		return
	}

	p.sessionMu.Lock()
	p.session = session
	p.sessionMu.Unlock()

	p.refresh()
}

func (p *Preview) refresh() {
	p.sessionMu.RLock()

	if p.session == nil {
		p.sessionMu.RUnlock()
		return
	}

	// get all windows for this session
	windows, _ := p.session.ListWindows()

	p.sessionMu.RUnlock()

	// collect all previews (one per pane), tracking the active pane
	// in the active window so we can snap the preview to it.
	previews := make([]string, 0)
	activeIdx := 0
	idx := 0
	for _, w := range windows {
		panes, _ := w.ListPanes()
		for _, pane := range panes {
			if w.Active && pane.Active {
				activeIdx = idx
			}
			preview, _ := pane.Capture()
			previews = append(previews, preview)
			idx++
		}
	}

	p.setPreviews(previews)
	p.setPreviewIdx(activeIdx)
}

func (p *Preview) render() {
	p.previewMu.RLock()
	defer p.previewMu.RUnlock()

	p.view.Clear()
	vp := getViewPosition(p.view.Name())
	if vp != nil {
		a.ui.Resize(p.view, vp)
	}

	if len(p.previews) == 0 {
		p.view.Subtitle = ""
		return
	}

	s := p.previews[p.previewIdx]
	p.view.Subtitle = fmt.Sprintf(" %d / %d ", min(p.previewIdx+1, len(p.previews)), len(p.previews))
	fmt.Fprintln(p.view, s)
}

func (p *Preview) setPreviews(previews []string) {
	p.previewMu.Lock()
	defer p.previewMu.Unlock()

	if len(previews) == 0 {
		p.previewIdx = 0
		p.previews = previews
		return
	}

	if p.previewIdx >= len(previews) {
		p.previewIdx = len(previews) - 1
	}
	p.previews = previews
}

// setPreviewIdx snaps the preview index to the given value, clamped to
// the current preview list. Called by refresh() to follow the active pane.
func (p *Preview) setPreviewIdx(idx int) {
	p.previewMu.Lock()
	defer p.previewMu.Unlock()

	if len(p.previews) == 0 {
		p.previewIdx = 0
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(p.previews) {
		idx = len(p.previews) - 1
	}
	p.previewIdx = idx
}

func (p *Preview) increment() {
	p.previewMu.Lock()
	defer p.previewMu.Unlock()
	if p.previewIdx == len(p.previews)-1 {
		p.previewIdx = 0
	} else {
		p.previewIdx++
	}
}

func (p *Preview) decrement() {
	p.previewMu.Lock()
	defer p.previewMu.Unlock()
	if p.previewIdx == 0 {
		p.previewIdx = len(p.previews) - 1
	} else {
		p.previewIdx--
	}
}

func (p *Preview) teardown() {
	p.done <- true
	a.ui.DeleteView(p.view)
}
