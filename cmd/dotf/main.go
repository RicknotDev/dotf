// DOTF — Zero-Configuration Linux Setup Runtime
//
// DOTF automatically detects your Linux environment and installs the
// correct configuration files from a layered repository.
// No manual profile selection, no symlink management, no documentation required.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codebuff/dotf/internal/cli"
)

const stateBaseDir = ".local/state"

func main() {
	prog := filepath.Base(os.Args[0])

	// Determine state directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
		os.Exit(1)
	}
	stateDir := filepath.Join(homeDir, stateBaseDir)

	if len(os.Args) < 2 {
		printUsage(prog)
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "install":
		err = cli.Install(args, stateDir)
	case "explain":
		err = cli.Explain(args)
	case "doctor":
		err = cli.Doctor(args, stateDir)
	case "inspect":
		err = cli.Inspect(args, stateDir)
	case "restore":
		err = cli.Restore(args, stateDir)
	case "help", "--help", "-h":
		printUsage(prog)
	case "version", "--version", "-v":
		printVersion(prog)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage(prog)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage(prog string) {
	fmt.Fprintf(os.Stderr, `DOTF — Zero-Configuration Linux Setup Runtime

Usage:
  %s install          Detect environment and install dotfiles
  %s explain          Show detected environment and layer decisions
  %s doctor           Run diagnostics and repair issues
  %s inspect          Deep inspection of files, layers, and state
  %s restore          Restore files from backups
  %s help             Show this help message
  %s version          Show version information

Examples:
  cd ~/.dotfiles && %s install
  cd ~/.dotfiles && %s explain
  cd ~/.dotfiles && %s doctor --fix
  cd ~/.dotfiles && %s restore --all

For more information: https://github.com/codebuff/dotf
`, prog, prog, prog, prog, prog, prog, prog, prog, prog, prog, prog)
}

func printVersion(prog string) {
	fmt.Printf("%s v0.3.0\n", prog)
}
