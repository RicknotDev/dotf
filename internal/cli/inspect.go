package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RicknotDev/dotf/internal/backup"
	"github.com/RicknotDev/dotf/internal/deploy"
	"github.com/RicknotDev/dotf/internal/layer"
	"github.com/RicknotDev/dotf/internal/profile"
	"github.com/RicknotDev/dotf/internal/state"
)

const inspectHelp = `Usage: dotf inspect <subcommand> [options]

Inspect DOTF internals. Uses the current directory, or the repository of
the last install.

Subcommands:
  file <path>          Show which layers provide a file and what is installed
  layer <name>         Show all files in a layer (e.g. distro/arch)
  state                Show the recorded state
  overrides            Show every resolved file and the layer that wins
  backup               Show the backups

Options:
  --profile <name>     Use a profile from dotf.yaml
  --json               Output in JSON format (global flag)
`

// Inspect runs the inspect command.
func Inspect(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("inspect")
	profileName := fs.String("profile", "", "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), inspectHelp)
		return nil
	}
	remaining := fs.Args()
	if len(remaining) < 1 {
		return usageErr("inspect requires a subcommand: file, layer, state, overrides, or backup")
	}
	subcmd, subargs := remaining[0], remaining[1:]

	switch subcmd {
	case "state":
		return inspectState(cfg, stateDir)
	case "backup":
		return inspectBackup(cfg, stateDir)
	case "file", "layer", "overrides":
	default:
		return usageErr("unknown inspect subcommand: %s (use: file, layer, state, overrides, backup)", subcmd)
	}

	repoRoot, err := requireRepo(stateDir)
	if err != nil {
		return err
	}
	switch subcmd {
	case "file":
		return inspectFile(cfg, repoRoot, stateDir, *profileName, subargs)
	case "layer":
		return inspectLayer(cfg, repoRoot, subargs)
	default:
		return inspectOverrides(cfg, repoRoot, *profileName)
	}
}

func inspectFile(cfg OutputConfig, repoRoot, stateDir, profileName string, args []string) error {
	if len(args) < 1 {
		return usageErr("file path required")
	}
	rel := filepath.ToSlash(filepath.Clean(args[0]))
	sel, err := selectLayers(repoRoot, profileName)
	if err != nil {
		return err
	}
	files, err := deploy.Collect(sel.Result.Layers, true)
	if err != nil {
		return err
	}

	type provider struct {
		Layer string `json:"layer"`
		Path  string `json:"path"`
	}
	var providers []provider
	for _, l := range sel.Result.Layers {
		p := filepath.Join(l.DirPath, rel)
		if _, err := os.Lstat(p); err == nil && !layer.Reserved(rel, false) {
			providers = append(providers, provider{l.Path(), p})
		}
	}
	winner := ""
	if e, ok := files[rel]; ok {
		winner = e.Layer
	}

	home, err := homeDir()
	if err != nil {
		return err
	}
	installed := map[string]string{"status": "not installed"}
	if info, err := os.Lstat(filepath.Join(home, rel)); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(filepath.Join(home, rel))
			installed = map[string]string{"status": "installed", "type": "symlink", "target": target}
		} else {
			installed = map[string]string{"status": "installed", "type": "regular file"}
		}
	}
	if sm, err := state.NewManager(stateDir); err == nil {
		if ref, ok := sm.File(rel); ok {
			installed["managed_by_dotf"] = "yes (" + ref.Type + " from " + ref.Layer + ")"
		}
	}

	if cfg.JSON {
		cfg.PrintJSON(map[string]interface{}{"file": rel, "providers": providers, "winner": winner, "installed": installed})
		return nil
	}
	cfg.Printf("Inspecting file: %s\n\n", rel)
	if len(providers) == 0 {
		cfg.Print("  (not found in any active layer)")
	}
	for _, p := range providers {
		mark := ""
		if p.Layer == winner {
			mark = "  <- wins"
		}
		cfg.Printf("  Layer:  %s%s\n  Path:   %s\n", p.Layer, mark, p.Path)
	}
	cfg.Print("")
	cfg.Print("Installed status:")
	keys := make([]string, 0, len(installed))
	for k := range installed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cfg.Printf("  %s: %s\n", k, installed[k])
	}
	return nil
}

func inspectLayer(cfg OutputConfig, repoRoot string, args []string) error {
	if len(args) < 1 {
		return usageErr("layer name required (e.g., distro/arch)")
	}
	lt, name, err := profile.ParseLayerPath(args[0])
	if err != nil {
		return err
	}
	rl := layer.ResolvedLayer{Type: lt, Name: name, DirPath: filepath.Join(repoRoot, "layers", "base")}
	if lt != profile.LayerBase {
		rl.DirPath = filepath.Join(repoRoot, "layers", string(lt), name)
	}
	info, err := os.Stat(rl.DirPath)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("layer not found: %s (%s)", args[0], rl.DirPath)
	}
	files, err := deploy.Collect([]layer.ResolvedLayer{rl}, true)
	if err != nil {
		return err
	}
	list := make([]string, 0, len(files))
	for rel, e := range files {
		if e.Kind == deploy.KindSecret {
			rel += " (secret)"
		}
		list = append(list, rel)
	}
	sort.Strings(list)

	if cfg.JSON {
		cfg.PrintJSON(map[string]interface{}{"layer": rl.Path(), "path": rl.DirPath, "files": list})
		return nil
	}
	cfg.Printf("Layer: %s\nPath:  %s\n\nFiles:\n", rl.Path(), rl.DirPath)
	for _, f := range list {
		cfg.Printf("  %s\n", f)
	}
	cfg.Printf("\nTotal: %s\n", plural(len(list), "file"))
	return nil
}

func inspectState(cfg OutputConfig, stateDir string) error {
	sm, err := state.NewManager(stateDir)
	if err != nil {
		return fmt.Errorf("cannot open state: %w", err)
	}
	s := sm.GetState()
	if cfg.JSON {
		cfg.PrintJSON(s)
		return nil
	}
	cfg.Print("DOTF State")
	cfg.Print("==========")
	cfg.Printf("  Version:        %d\n", s.Version)
	cfg.Printf("  Repository:     %s\n", nonEmpty(s.Repository, "-"))
	cfg.Printf("  Last install:   %s\n\n", nonEmpty(s.LastInstall, "never"))

	cfg.Printf("Installed layers (%d):\n", len(s.InstalledLayers))
	for _, l := range s.InstalledLayers {
		cfg.Printf("  %s\n", l)
	}
	cfg.Printf("\nInstalled files (%d):\n", len(s.InstalledFiles))
	for _, rel := range sm.Files() {
		ref := s.InstalledFiles[rel]
		cfg.Printf("  %s [%s] from %s\n", rel, ref.Type, ref.Layer)
	}
	paths := make([]string, 0, len(s.BackupManifest))
	for p := range s.BackupManifest {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	cfg.Printf("\nBackups (%d):\n", len(paths))
	for _, p := range paths {
		b := s.BackupManifest[p]
		cfg.Printf("  %s -> %s (%s)\n", p, b.Original, b.Created)
	}
	return nil
}

// inspectOverrides lists every resolved file. Layers are walked from lowest
// to highest priority, so the last provider is the one that is installed.
func inspectOverrides(cfg OutputConfig, repoRoot, profileName string) error {
	sel, err := selectLayers(repoRoot, profileName)
	if err != nil {
		return err
	}
	files, err := deploy.Collect(sel.Result.Layers, true)
	if err != nil {
		return err
	}
	rels := make([]string, 0, len(files))
	for r := range files {
		rels = append(rels, r)
	}
	sort.Strings(rels)

	if cfg.JSON {
		type entry struct {
			File       string   `json:"file"`
			Winner     string   `json:"winner"`
			Overridden []string `json:"overridden,omitempty"`
			Secret     bool     `json:"secret,omitempty"`
		}
		out := []entry{}
		for _, r := range rels {
			e := files[r]
			out = append(out, entry{r, e.Layer, e.Overridden, e.Kind == deploy.KindSecret})
		}
		cfg.PrintJSON(out)
		return nil
	}

	cfg.Print("File Overrides")
	cfg.Print("==============")
	cfg.Print("")
	cfg.Printf("%s", layer.FormatOverrideGroups(layer.ComputeOverrides(sel.Result.Layers)))
	cfg.Print("")
	cfg.Print("All resolved files:")
	for _, r := range rels {
		e := files[r]
		line := fmt.Sprintf("  %s <- %s", r, e.Layer)
		if len(e.Overridden) > 0 {
			line += " (overrides: " + strings.Join(e.Overridden, ", ") + ")"
		}
		if e.Kind == deploy.KindSecret {
			line += " (secret)"
		}
		cfg.Print(line)
	}
	return nil
}

func inspectBackup(cfg OutputConfig, stateDir string) error {
	bm, err := backup.NewManager(filepath.Join(stateDir, "backups"))
	if err != nil {
		return err
	}
	groups, err := bm.ListAll()
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if cfg.JSON {
		cfg.PrintJSON(groups)
		return nil
	}
	cfg.Print("Backup Manifest")
	cfg.Print("===============")
	cfg.Print("")
	if len(keys) == 0 {
		cfg.Print("  (no backups found)")
		return nil
	}
	for _, k := range keys {
		cfg.Printf("  %s:\n", k)
		for _, b := range groups[k] {
			size := int64(0)
			if info, err := os.Lstat(b.BackupPath); err == nil {
				size = info.Size()
			}
			cfg.Printf("    %s  %d bytes  %s\n", b.Time.Format("2006-01-02 15:04:05"), size, filepath.Base(b.BackupPath))
		}
	}
	return nil
}
