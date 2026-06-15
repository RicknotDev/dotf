package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/codebuff/dotf/internal/detect"
	"github.com/codebuff/dotf/internal/layer"
)

// Explain runs the explain command.
func Explain(args []string) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Fprint(os.Stderr, `Usage: dotf explain

Display the detected environment, resolved layers, and any file overrides.

This command helps you understand exactly which configuration files
DOTF will use and which layers take priority.
`)
		return nil
	}

	// Determine the repository root
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot determine working directory: %w", err)
	}

	// Detect environment
	p := detect.Detect()

	// Print detected environment
	fmt.Println(p.String())
	fmt.Println()

	// Resolve layers
	result, err := layer.Resolve(repoRoot, p)
	if err != nil {
		return fmt.Errorf("resolving layers: %w", err)
	}

	fmt.Println("Activated Layers:")
	if len(result.Layers) == 0 {
		fmt.Println("  (none)")
	} else {
		for _, l := range result.Layers {
			fmt.Printf("  %s\n", l.Path())
		}
	}
	fmt.Println()

	if len(result.Missing) > 0 {
		fmt.Println("Unavailable Layers (not found in repository):")
		for _, l := range result.Missing {
			fmt.Printf("  %s\n", l.Path())
		}
		fmt.Println()
	}

	// Compute and display overrides
	overrides := layer.ComputeOverrides(result.Layers)
	fmt.Println("Overrides:")
	fmt.Print(layer.FormatOverrideGroups(overrides))

	return nil
}
