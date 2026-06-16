package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codebuff/dotf/internal/cli"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Determine state directory (XDG compliant)
	stateDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "dotf")

	switch cmd {
	case "install":
		if err := cli.Install(args, stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "explain":
		if err := cli.Explain(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "doctor":
		if err := cli.Doctor(args, stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "inspect":
		if err := cli.Inspect(args, stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "restore":
		if err := cli.Restore(args, stateDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `Usage: dotf <command> [options]

Commands:
  install    Install dotfiles to your home directory
  explain    Display detected environment and layer decisions
  doctor     Run diagnostics and repair
  inspect    Inspect internals (file, layer, state, overrides, backup)
  restore    Restore files from backups

Run 'dotf <command> --help' for detailed usage.
`)
}
