package core

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Data for the local config store.
type LocalConfigData struct {
	SelectedWorkspace string            `json:"selected-workspace"`
	SessionComments   map[string]string `json:"session-comments,omitempty"`
	// WorktreeRoot, when set, opts the user into worktree → session
	// auto-sync: any directory directly under this path that is itself
	// a git worktree gets a tmux session created for it. Sessions whose
	// backing worktree disappears render as pending in the grid; mynav
	// never kills sessions on its own.
	WorktreeRoot string `json:"worktree-root,omitempty"`
	// SessionOrder is a user-curated ordering of tmux session names,
	// produced by the move-mode UI in the Sessions view. Sessions in
	// this list sort first, in their listed order; sessions absent
	// from it fall through to the created-time fallback. Stale names
	// (sessions that have been killed) are tolerated and replaced
	// whole on the next move-mode commit.
	SessionOrder []string `json:"session-order,omitempty"`
}

// LocalConfig is the LocalConfig configuration.
type LocalConfig struct {
	datasource *Datasource[LocalConfigData]
	path       string
}

func newLocalConfig(dir string) (*LocalConfig, error) {
	c := &LocalConfig{}
	// if dir is passed we initialize it and dont detect
	if dir != "" {
		// check if dir is home dir
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		if home == dir {
			return nil, errors.New("mynav cannot be initialized in the home directory")
		}

		// set up dir
		if err := c.setupDir(dir); err != nil {
			return nil, err
		}

		// set up datasource in the dir
		if err := c.setupDatasource(dir); err != nil {
			return nil, err
		}
		return c, nil
	}

	// if dir is not passed we detect
	path, err := c.detect()
	if err != nil {
		return nil, err
	}

	// return no error and nil if no config here
	if path == "" {
		return nil, nil
	}

	// if config exists set up datasource
	if err := c.setupDatasource(path); err != nil {
		return nil, err
	}

	return c, nil
}

func (l *LocalConfig) setupDatasource(rootdir string) error {
	ds, err := newDatasource(filepath.Join(rootdir, ".mynav", "config.json"), &LocalConfigData{})
	if err != nil {
		return err
	}

	l.path = rootdir
	l.datasource = ds
	return nil
}

func (c *LocalConfig) setupDir(rootdir string) error {
	path := filepath.Join(rootdir, ".mynav")
	return CreateDir(path)
}

func (c *LocalConfig) detect() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Panicln(err)
	}
	dirEntries := GetDirEntries(cwd)
	homeDir, _ := os.UserHomeDir()

	for {
		for _, entry := range dirEntries {
			if cwd == "/" {
				return "", nil
			}
			if entry.Name() == ".mynav" {
				if cwd == homeDir {
					break
				}

				if !entry.IsDir() {
					os.Remove(filepath.Join(cwd, entry.Name()))
					c.setupDir(cwd)
					return cwd, nil
				}

				return cwd, nil
			}
		}
		cwd = filepath.Dir(cwd)
		dirEntries = GetDirEntries(cwd)
	}
}

func (g *LocalConfig) SetSelectedWorkspace(s string) {
	data := g.datasource.Get()
	data.SelectedWorkspace = s
	g.datasource.Save(data)
}

// SessionComment returns the saved comment for the named tmux session, or "".
func (g *LocalConfig) SessionComment(name string) string {
	data := g.datasource.Get()
	if data.SessionComments == nil {
		return ""
	}
	return data.SessionComments[name]
}

// SetSessionComment persists a comment for the named tmux session. An empty
// comment removes the entry.
func (g *LocalConfig) SetSessionComment(name, comment string) {
	data := g.datasource.Get()
	if data.SessionComments == nil {
		data.SessionComments = map[string]string{}
	}
	if comment == "" {
		delete(data.SessionComments, name)
	} else {
		data.SessionComments[name] = comment
	}
	g.datasource.Save(data)
}

func (l *LocalConfig) ConfigData() *LocalConfigData {
	return l.datasource.Get()
}

// WorktreeRoot returns the configured worktree root (absolute path),
// or "" if the user has not opted in.
func (l *LocalConfig) WorktreeRoot() string {
	return l.datasource.Get().WorktreeRoot
}

// SessionOrder returns the user-curated session ordering (tmux names).
// Returns nil when the user has never reordered.
func (l *LocalConfig) SessionOrder() []string {
	return l.datasource.Get().SessionOrder
}

// SetSessionOrder persists the user-curated session ordering. Caller
// supplies the full list of currently visible tmux names in the order
// the user wants — stale entries are simply overwritten.
func (l *LocalConfig) SetSessionOrder(order []string) {
	data := l.datasource.Get()
	data.SessionOrder = order
	l.datasource.Save(data)
}

func isBeforeOneHourAgo(timestamp time.Time) bool {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)
	return timestamp.Before(oneHourAgo)
}
