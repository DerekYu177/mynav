package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	"github.com/GianlucaP106/mynav/pkg/core"
	"github.com/GianlucaP106/mynav/pkg/tui"
	"github.com/awesome-gocui/gocui"
	"github.com/gookit/color"
)

type App struct {
	// api instance
	api *core.API

	// ui instance (wrapper over gocui)
	ui *tui.TUI

	// views
	sessions *Sessions
	preview  *Preview
	details  *Details

	// worker for processing tasks in FIFO and debouncing
	worker *Worker

	// if the app ui is first initialized
	initialized atomic.Bool

	// if a session is currently attached (or yielding to another process)
	// background workers can use this to avoid consuming ressources
	attached atomic.Bool
}

// worker magic numbers
const (
	// size of the worker queue
	defaultWorkerSize = 100

	// time to debounce for worker
	defaultWorkerDebounce = 200 * time.Millisecond
)

// view styles
const (
	onFrameColor  = gocui.ColorWhite
	offFrameColor = gocui.AttrDim | gocui.ColorWhite
	onTitleColor  = gocui.AttrBold | gocui.ColorGreen
	offTitleColor = gocui.AttrBold | gocui.ColorCyan
)

// text styles
var (
	topicNameColor              = color.New(color.FgYellow, color.Bold)
	workspaceNameColor          = color.New(color.FgBlue, color.Bold)
	timestampColor              = color.New(color.FgDarkGray, color.OpItalic)
	sessionMarkerColor          = color.New(color.FgGreen, color.Bold)
	alternateSessionMarkerColor = color.New(color.Magenta, color.Bold)
)

// global a instance
var a *App

// Inits and starts the app.
func Start() {
	a = newApp()
	a.start()
}

// Inits the app.
func newApp() *App {
	a := &App{}
	return a
}

// Starts the app.
func (a *App) start() {
	// run cli and handle args
	newCli().run()

	// init start refresh queue
	a.worker = newWorker(200*time.Millisecond, defaultWorkerSize)
	go a.worker.Start()

	// init the app
	a.init()

	// run main loop
	defer a.ui.Close()
	err := a.ui.MainLoop()
	if err != nil {
		if !errors.Is(err, gocui.ErrQuit) {
			log.Panicln(err)
		}
	}
}

// Inits the app (api, tui, views).
func (a *App) init() {
	// define small helper functions
	initApp := func() {
		// initialize UI
		a.initUI()

		// refresh (populate data to the views)
		a.refreshInit()

		// update toast
		available, tag := a.api.UpdateAvailable()
		if available {
			toast(fmt.Sprintf("mynav %s is available", tag), toastWarn)
		}
	}
	close := func() {
		// start closing after 3 seconds, and display the close counter for 6
		a.closeAfter(6, 3*time.Second)
	}

	// init tui
	a.ui = tui.NewTui()

	// init temp ui to ask for initialization and report errors
	a.tempUI()

	// init api
	var err error
	a.api, err = core.NewApi("")
	if err != nil {
		toast(err.Error(), toastError)
		close()
		return
	}

	// if api is initialized then we can initialize the app
	if a.api != nil {
		initApp()
		return
	}

	// get current dir
	curDir, err := os.Getwd()
	if err != nil {
		toast(err.Error(), toastError)
		close()
		return
	}

	// ensure current dir is not home directory
	home, err := os.UserHomeDir()
	if err != nil {
		toast(err.Error(), toastError)
		close()
		return
	}
	if home == curDir {
		toast("mynav cannot be initialized in the home directory, closing...", toastError)
		close()
		return
	}

	// ask to initalize, and handle error cases
	alert(func(b bool) {
		if !b {
			toast("mynav needs a directory to initialize", toastError)
			close()
			return
		}

		// reinit the api in this dir
		a.api, err = core.NewApi(curDir)
		if err != nil {
			toast(err.Error(), toastError)
			close()
			return
		}

		// handle nil just in case (should not be nil again)
		if a.api == nil {
			toast("Could not initialize mynav", toastError)
			close()
			return
		}

		// finally initialize
		initApp()
	}, "No configuration found. Would you like to initialize this directory?")
}

// Inits the UI, views.
func (a *App) initUI() {
	// instantiate views
	pv := newPreview()
	sv := newSessionsView()
	dv := newDetails()
	a.sessions = sv
	a.preview = pv
	a.details = dv

	// set manager functions that render the views. The details overlay
	// renders LAST so it sits on top of the preview's top-right corner.
	a.ui.SetManager(func(t *tui.TUI) error {
		sv.render()
		pv.render()
		dv.render()
		return nil
	})

	// init the views (configs, actions etc...)
	sv.init()
	pv.init(a.ui.SetView(getViewPosition(PreviewView)))

	// set global key bindings
	a.initGlobalKeys()
}

// Initializes a temporary (incomplete) ui for initialization.
func (a *App) tempUI() {
	// set a manager that runs no renders
	a.ui.SetManager(func(t *tui.TUI) error {
		return nil
	})

	// set only quit keymaps
	quit := func() bool {
		return true
	}
	a.ui.KeyBinding(nil).
		SetWithQuit(gocui.KeyCtrlC, quit, "Quit").
		SetWithQuit('q', quit, "Quit").
		SetWithQuit('q', quit, "Quit")
}

// Focuses a given view by also changing styles.
func (a *App) focusView(view *tui.View) {
	a.ui.FocusView(view)

	// only the sessions view is focusable now
	if a.sessions != nil && a.sessions.view != nil {
		if a.sessions.view.Name() == view.Name() {
			a.sessions.view.FrameColor = onFrameColor
			a.sessions.view.TitleColor = onTitleColor
		} else {
			a.sessions.view.FrameColor = offFrameColor
			a.sessions.view.TitleColor = offTitleColor
		}
	}
}

// Applies general styles to view.
func (a *App) styleView(v *tui.View) {
	v.TitleColor = offTitleColor
	v.FrameColor = offFrameColor
	v.FrameRunes = tui.ThinFrame
}

// Refreshes all the views.
// If selectSession is not nil, it will be selected in the sessions view.
func (a *App) refresh(selectSession *core.Session) {
	a.worker.Queue(func() {
		// sessions in async
		go func() {
			a.sessions.refresh()
			if selectSession != nil {
				a.sessions.selectSession(selectSession)
				a.sessions.refreshPreview()
				a.details.show(selectSession)
			}
			a.ui.Update(func() {
				if selectSession != nil {
					a.preview.render()
				}
				a.sessions.render()
			})
		}()
	})
}

// Modified version of refresh designed to run on start up.
// Sets loading flags and focuses the sessions view.
func (a *App) refreshInit() {
	sv := a.sessions

	a.worker.Queue(func() {
		// sessions in async
		a.ui.Update(func() {
			sv.setLoading(true)
		})
		sv.refresh()
		a.ui.Update(func() {
			sv.setLoading(false)
			sv.render()
		})

		// pick a session to focus on
		selected := a.api.SelectedWorkspace()
		var selectedSession *core.Session
		if selected != nil {
			selectedSession = a.api.Session(selected)
			if selectedSession != nil {
				sv.selectSession(selectedSession)
			}
		}

		sv.refreshPreview()
		a.details.show(sv.selected())
		a.ui.Update(func() {
			a.preview.render()
			sv.focus()
			a.initialized.Store(true)
		})
	})
}

// Runs f in between a tui suspend-resume allowing other terminal apps to run.
func (a *App) runAction(f func() error) error {
	a.attached.Store(true)
	tui.Suspend()
	err := f()
	tui.Resume()
	a.attached.Store(false)
	return err
}

// Closes the app after count seconds and displays a ticker as a toast.
func (a *App) closeAfter(count int, delay time.Duration) {
	time.AfterFunc(delay, func() {
		ticker := time.NewTicker(time.Second)
		for range ticker.C {
			if count == 0 {
				a.ui.Close()
				os.Exit(0)
			}
			a.ui.Update(func() {
				toast(fmt.Sprintf("Closing in %d seconds...", count), toastWarn)
			})
			count--
		}
	})
}

// Inits the global actions.
func (a *App) initGlobalKeys() {
	quit := func() bool {
		return true
	}
	a.ui.KeyBinding(nil).
		SetWithQuit(gocui.KeyCtrlC, quit, "Quit").
		SetWithQuit('q', quit, "Quit").
		SetWithQuit('q', quit, "Quit").
		Set('?', "Toggle cheatsheet", func() {
		}).
		Set('<', "Cycle preview left", func() {
			a.preview.decrement()
		}).
		Set('>', "Cycle preview right", func() {
			a.preview.increment()
		}).
		Set('s', "Search", func() {
			// block if not initialized to avoid broken state
			if !a.initialized.Load() {
				return
			}

			newGlobalSearch().init()
		})
}
