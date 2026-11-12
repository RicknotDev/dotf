package cli

import (
	"fmt"

	"github.com/RicknotDev/dotf/internal/layer"
)

const explainHelp = `Usage: dotf explain [--profile <name>]

Display the detected environment, the layers that are active, and which
layer wins for files present in several layers.

Uses the current directory, or the repository of the last install.
`

// Explain runs the explain command.
func Explain(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("explain")
	profileName := fs.String("profile", "", "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), explainHelp)
		return nil
	}

	repoRoot, _, err := findRepo(stateDir)
	if err != nil {
		return err
	}
	sel, err := selectLayers(repoRoot, *profileName)
	if err != nil {
		return err
	}
	for _, w := range sel.Warnings {
		cfg.Warn("%s", w)
	}
	result := sel.Result
	overrides := layer.ComputeOverrides(result.Layers)

	if cfg.JSON {
		type override struct {
			File       string `json:"file"`
			Winner     string `json:"winner"`
			Overridden string `json:"overridden"`
		}
		ov := []override{}
		for _, o := range overrides {
			ov = append(ov, override{o.File, o.Overriding, o.Overridden})
		}
		cfg.PrintJSON(map[string]interface{}{
			"repository": repoRoot,
			"profile":    sel.ProfileName,
			"detected":   profileFields(sel),
			"layers":     getLayerPaths(result.Layers),
			"missing":    missingPaths(result),
			"overrides":  ov,
		})
		return nil
	}

	cfg.Print(sel.Profile.String())
	if sel.ProfileName != "" {
		cfg.Printf("Profile: %s\n\n", sel.ProfileName)
	}
	cfg.Print("Activated Layers:")
	if len(result.Layers) == 0 {
		cfg.Print("  (none)")
	}
	for _, l := range result.Layers {
		cfg.Printf("  %s\n", l.Path())
	}
	cfg.Print("")
	if len(result.Missing) > 0 {
		cfg.Print("Unavailable Layers (not found in repository):")
		for _, l := range result.Missing {
			cfg.Printf("  %s\n", l.Path())
		}
		cfg.Print("")
	}
	cfg.Print("Overrides:")
	cfg.Printf("%s", layer.FormatOverrideGroups(overrides))
	return nil
}
