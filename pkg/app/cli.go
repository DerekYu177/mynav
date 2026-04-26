package app

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/GianlucaP106/mynav/pkg/core"
)

type (
	Cli struct {
		args *CliArgs
	}
	CliArgs struct {
		version *bool
		path    *string
	}
)

func newCli() *Cli {
	return &Cli{}
}

func (cli *Cli) run() {
	// Subcommands run before flag.Parse so `mynav hook <event>`
	// (invoked by Claude Code) doesn't accidentally start the TUI.
	if handleSubcommand(os.Args[1:]) {
		os.Exit(0)
	}
	cli.parseArgs()
	cli.handleVersionFlag()
	cli.handlePathFlag()
}

// handleSubcommand dispatches positional subcommands. Returns true
// when a subcommand ran and the caller should exit.
func handleSubcommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "hook":
		runHookCommand(args[1:])
		return true
	case "install-hooks":
		runInstallHooks()
		return true
	case "uninstall-hooks":
		runUninstallHooks()
		return true
	}
	return false
}

func runInstallHooks() {
	bin, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "install-hooks: cannot resolve own path:", err)
		os.Exit(1)
	}
	if err := core.InstallHooks(bin); err != nil {
		fmt.Fprintln(os.Stderr, "install-hooks:", err)
		os.Exit(1)
	}
	path, _ := core.ClaudeSettingsPath()
	fmt.Printf("installed mynav hooks into %s\n", path)
}

func runUninstallHooks() {
	if err := core.UninstallHooks(); err != nil {
		fmt.Fprintln(os.Stderr, "uninstall-hooks:", err)
		os.Exit(1)
	}
	path, _ := core.ClaudeSettingsPath()
	fmt.Printf("removed mynav hooks from %s\n", path)
}

func (cli *Cli) parseArgs() {
	version := flag.Bool("version", false, "Version of mynav")
	path := flag.String("path", ".", "Path to open mynav in")
	flag.Parse()
	cli.args = &CliArgs{
		version: version,
		path:    path,
	}
}

func (cli *Cli) handleVersionFlag() {
	if *cli.args.version {
		fmt.Println(core.Version)
		os.Exit(0)
	}
}

func (cli *Cli) handlePathFlag() {
	if cli.args.path != nil && *cli.args.path != "" {
		if err := os.Chdir(*cli.args.path); err != nil {
			log.Fatalln(err.Error())
		}
	}
}
