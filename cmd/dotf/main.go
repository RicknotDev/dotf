package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/RicknotDev/dotf/internal/cli"
)

// Version is set at build time via -ldflags "-X main.Version=v0.7.0".
var Version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one dotf invocation and returns the exit code.
//
// Exit codes: 0 success, 1 error, 2 unresolved conflict, 3 nothing to do,
// 64 usage error.
func run(args []string, stdout, stderr io.Writer) int {
	cfg, rest := cli.ParseGlobalFlags(args)
	cfg.Stdout, cfg.Stderr = stdout, stderr
	if f, ok := stdout.(*os.File); !ok || !cli.IsTerminal(f) {
		cfg.NoColor = true
	}

	if len(rest) == 0 {
		printWelcome(stdout)
		return cli.ExitOK
	}
	cmd, cmdArgs := rest[0], rest[1:]
	stateDir := xdgStateDir()

	var err error
	switch cmd {
	case "install":
		err = cli.Install(cmdArgs, stateDir, cfg)
	case "explain":
		err = cli.Explain(cmdArgs, stateDir, cfg)
	case "doctor":
		err = cli.Doctor(cmdArgs, stateDir, cfg)
	case "inspect":
		err = cli.Inspect(cmdArgs, stateDir, cfg)
	case "restore":
		err = cli.Restore(cmdArgs, stateDir, cfg)
	case "status":
		err = cli.Status(cmdArgs, stateDir, cfg)
	case "apply":
		err = cli.Apply(cmdArgs, stateDir, cfg)
	case "unapply":
		err = cli.Unapply(cmdArgs, stateDir, cfg)
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return cli.ExitOK
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "dotf %s\n", Version)
		return cli.ExitOK
	default:
		cfg.PrintErr("Error: unknown command: %s\n\n", cmd)
		if !cfg.JSON {
			fmt.Fprint(stderr, usage)
		}
		return cli.ExitUsage
	}

	if err != nil {
		if cfg.JSON {
			cfg.PrintErr("%v", err)
		} else {
			cfg.PrintErr("Error: %v\n", err)
		}
	}
	return cli.ExitCode(err)
}

const commands = `Commands:
  install    Install dotfiles from a repository
  explain    Show detected environment and layer decisions
  status     Show installation status of dotfiles
  apply      Deploy the layers' files (no hooks, no packages)
  unapply    Remove the files dotf installed
  doctor     Run diagnostics and repair issues
  inspect    Inspect internals (file, layer, state, overrides, backup)
  restore    Restore files from backups
  version    Print the version

Global flags (anywhere on the command line):
  --json       Output in JSON format
  --quiet      Suppress non-error output
  --no-color   Disable colored output (also NO_COLOR)
  --filter     Filter status output: value or key:value

Run 'dotf <command> --help' for detailed usage.
`

const usage = "Usage: dotf <command> [options]\n\n" + commands

func printWelcome(w io.Writer) {
	const width = 44 // inner width of the box
	line := func(s string) string {
		pad := width - len([]rune(s))
		if pad < 0 {
			pad = 0
		}
		left := pad / 2
		return "║" + strings.Repeat(" ", left) + s + strings.Repeat(" ", pad-left) + "║\n"
	}
	border := strings.Repeat("═", width)
	fmt.Fprint(w, "╔"+border+"╗\n"+
		line("DOTF "+Version)+
		line("Zero-Configuration Dotfiles Runtime")+
		"╚"+border+"╝\n")
	fmt.Fprint(w, `
DOTF detects your Linux environment and installs the matching
configuration files from a layered repository.

Quick start:

  1. Clone or create your dotfiles repository:
     git clone https://github.com/you/dotfiles
     cd dotfiles

  2. See what DOTF detects about your system:
     dotf explain

  3. Preview, then install:
     dotf install --dry-run
     dotf install

`+commands)
}

// xdgStateDir returns the DOTF state directory following the XDG Base
// Directory spec: $XDG_STATE_HOME/dotf, or ~/.local/state/dotf.
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
