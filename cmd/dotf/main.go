package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codebuff/dotf/internal/cli"
)

// Version is set at build time via -ldflags -X main.Version=v0.6.0
var Version = "dev"

func main() {
	// Parse global flags and NO_COLOR early
	if len(os.Args) < 2 {
		printWelcome()
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Determine state directory (XDG compliant)
	stateDir := xdgStateDir()

	// Parse global flags for all commands
	globalCfg, cmdArgs := cli.ParseGlobalFlags(args)

	// Exit codes:
	// 0 = success
	// 1 = general error
	// 2 = conflict without resolution
	// 3 = nothing to do

	var err error
	switch cmd {
	case "install":
		err = cli.Install(cmdArgs, stateDir)
	case "explain":
		err = cli.Explain(cmdArgs)
	case "doctor":
		err = cli.Doctor(cmdArgs, stateDir)
	case "inspect":
		err = cli.Inspect(cmdArgs, stateDir)
	case "restore":
		err = cli.Restore(cmdArgs, stateDir)
	case "status":
		err = cli.Status(cmdArgs, stateDir, globalCfg)
	case "apply":
		err = cli.Apply(cmdArgs, stateDir, globalCfg)
	case "unapply":
		err = cli.Unapply(cmdArgs, stateDir, globalCfg)
	case "help", "--help", "-h":
		printUsage()
		os.Exit(0)
	case "version", "--version", "-v":
		fmt.Fprintf(os.Stderr, "dotf %s\n", Version)
		os.Exit(0)
	default:
		globalCfg.PrintErr("Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		// Map error types to exit codes
		exitCode := 1
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "already exists") {
			exitCode = 2
		} else if strings.Contains(err.Error(), "nothing to do") || strings.Contains(err.Error(), "no files") {
			exitCode = 3
		}
		// In JSON mode, the error was already reported as structured JSON by the command
		// Only print the error prefix for non-JSON output
		if !globalCfg.JSON {
			globalCfg.PrintErr("Error: %v\n", err)
		}
		os.Exit(exitCode)
	}
}

func printWelcome() {
	fmt.Fprintf(os.Stderr, `╔══════════════════════════════════════════╗
║          DOTF %-22s║
║  Zero-Configuration Dotfiles Runtime    ║
╚══════════════════════════════════════════╝

DOTF automatically detects your Linux environment and installs
the correct configuration files from a layered repository.

Quick start in 3 steps:

  1. Clone or create your dotfiles repository:
     git clone https://github.com/you/dotfiles
     cd dotfiles

  2. See what DOTF detects about your system:
     dotf explain

  3. Install your dotfiles:
     dotf install

Commands:
  install    Install dotfiles from a repository
  explain    Show detected environment and layer decisions
  status     Show installation status of dotfiles
  apply      Apply a profile
  unapply    Revert applied changes
  doctor     Run diagnostics and repair issues
  inspect    Inspect internals (file, layer, state, overrides)
  restore    Restore files from backups

Global flags:
  --json       Output in JSON format (scriptable)
  --quiet      Suppress non-error output
  --no-color   Disable colored output
  --filter     Filter output by expression

Run 'dotf <command> --help' for detailed usage.
`, Version)
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: dotf <command> [options]

Commands:
  install    Install dotfiles to your home directory
  explain    Display detected environment and layer decisions
  status     Show installation status of dotfiles
  apply      Apply a profile
  unapply    Revert applied changes
  doctor     Run diagnostics and repair
  inspect    Inspect internals (file, layer, state, overrides, backup)
  restore    Restore files from backups

Global flags:
  --json       Output in JSON format
  --quiet      Suppress non-error output
  --no-color   Disable colored output
  --filter     Filter output by expression

Run 'dotf <command> --help' for detailed usage.
`)
}

// xdgStateDir returns the DOTF state directory following the XDG Base Directory spec.
// Uses $XDG_STATE_HOME/dotf if set, otherwise falls back to ~/.local/state/dotf.
func xdgStateDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "dotf")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Last resort fallback
		return filepath.Join("/tmp", "dotf-state")
	}
	return filepath.Join(home, ".local", "state", "dotf")
}
