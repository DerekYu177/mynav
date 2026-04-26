package app

import (
	"os"

	"github.com/GianlucaP106/mynav/pkg/core"
)

// runHookCommand implements `mynav hook <event>`. Stdin is the JSON
// payload Claude Code passes to its hooks; we marshal it to a queue
// file under core.QueueDir() for the running mynav UI to read.
//
// Errors are deliberately swallowed: hooks must never block Claude
// Code, and the worst case is that one event is missed and pattern
// matching fills in.
func runHookCommand(args []string) {
	if len(args) == 0 {
		return
	}
	_ = core.WriteHookEvent(args[0], os.Stdin)
}
