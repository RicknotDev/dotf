package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/RicknotDev/dotf/internal/backup"
	"github.com/RicknotDev/dotf/internal/deploy"
	"github.com/RicknotDev/dotf/internal/hook"
	"github.com/RicknotDev/dotf/internal/layer"
	"github.com/RicknotDev/dotf/internal/pkg"
	"github.com/RicknotDev/dotf/internal/reorganize"
	"github.com/RicknotDev/dotf/internal/state"
)

const installHelp = `Usage: dotf install [options] [path|git-url]

Detect the environment, resolve the matching layers and install their files
into your home directory.

Arguments:
  path|git-url    Dotfiles repository (default: current directory). A git URL
                  is cloned to the state directory and updated on later runs.

Options:
  --dry-run         Show what would be done; write nothing, run nothing
  --copy            Copy files instead of creating symlinks
  --profile <name>  Use a profile from dotf.yaml
  --allow-hooks     Run hook scripts (also DOTF_ALLOW_HOOKS=1)
  --no-packages     Do not install packages from packages/<manager>.txt
  --no-secrets      Do not deploy secrets from secrets/
  --reorganize      Convert a flat dotfiles directory into layers/ first
  --help            Show this help message
`

// Install runs the install command.
func Install(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("install")
	dryRun := fs.Bool("dry-run", false, "")
	copyMode := fs.Bool("copy", false, "")
	allowHooks := fs.Bool("allow-hooks", false, "")
	noPackages := fs.Bool("no-packages", false, "")
	noSecrets := fs.Bool("no-secrets", false, "")
	reorganizeFlag := fs.Bool("reorganize", false, "")
	profileName := fs.String("profile", "", "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), installHelp)
		return nil
	}
	if fs.NArg() > 1 {
		return usageErr("install takes at most one path or URL")
	}

	repoRoot, cleanup, err := resolveInstallSource(fs.Arg(0), stateDir, *dryRun, cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	if reorganize.IsFlatStructure(repoRoot) {
		if !*reorganizeFlag {
			cfg.Info("Detected a flat dotfiles directory. Run with --reorganize to convert it to the layers/ format.\n")
		} else {
			result, err := reorganize.Analyze(repoRoot)
			if err != nil {
				return fmt.Errorf("cannot analyze structure: %w", err)
			}
			if *dryRun {
				cfg.Info("Would reorganize into layers/:\n%s", reorganize.FormatResult(result))
				if !isRepo(repoRoot) {
					cfg.Info("\nDry run complete. Run without --dry-run to reorganize and install.\n")
					return nil
				}
			} else {
				if err := reorganize.Reorganize(repoRoot, result); err != nil {
					return fmt.Errorf("reorganization failed: %w", err)
				}
				cfg.Info("%s", reorganize.FormatResult(result))
			}
		}
	}
	if !isRepo(repoRoot) {
		return fmt.Errorf("not a DOTF repository: no 'layers' directory found in %s\n\nRun 'dotf install' from the root of your dotfiles repository, or use --reorganize to convert a flat dotfiles directory", repoRoot)
	}

	mode := deploy.Symlink
	if *copyMode {
		mode = deploy.Copy
	}
	return runDeploy(deployRequest{
		cfg:         cfg,
		stateDir:    stateDir,
		repoRoot:    repoRoot,
		profileFlag: *profileName,
		mode:        mode,
		dryRun:      *dryRun,
		hooks:       hook.Allowed(*allowHooks),
		packages:    !*noPackages,
		noSecrets:   *noSecrets,
		prune:       true,
		command:     "install",
	})
}

// resolveInstallSource returns the repository directory for a path or git
// URL argument. Git URLs are cloned under <stateDir>/repos and pulled on
// later runs; a dry run never touches the cache (it clones to a temporary
// directory when needed).
func resolveInstallSource(arg, stateDir string, dryRun bool, cfg OutputConfig) (string, func(), error) {
	noop := func() {}
	if arg == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", noop, fmt.Errorf("cannot determine working directory: %w", err)
		}
		return wd, noop, nil
	}
	if !isGitURL(arg) {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", noop, fmt.Errorf("cannot resolve path %s: %w", arg, err)
		}
		return abs, noop, nil
	}

	cloneDir := filepath.Join(stateDir, "repos", repoCacheName(arg))
	_, statErr := os.Stat(cloneDir)
	cached := statErr == nil
	switch {
	case cached && dryRun:
		cfg.Info("Using cached clone %s (not updated in dry run)\n", cloneDir)
		return cloneDir, noop, nil
	case cached:
		cfg.Info("Updating %s...\n", arg)
		if err := runGit(cfg, "-C", cloneDir, "pull", "--ff-only"); err != nil {
			return "", noop, fmt.Errorf("cannot update cached clone %s: %w", cloneDir, err)
		}
		return cloneDir, noop, nil
	case dryRun:
		tmp, err := os.MkdirTemp("", "dotf-dry-run-")
		if err != nil {
			return "", noop, err
		}
		cleanup := func() { os.RemoveAll(tmp) }
		dir := filepath.Join(tmp, "repo")
		cfg.Info("Cloning %s to a temporary directory...\n", arg)
		if err := runGit(cfg, "clone", "--depth", "1", "--", arg, dir); err != nil {
			cleanup()
			return "", noop, fmt.Errorf("cannot clone repository: %w", err)
		}
		return dir, cleanup, nil
	default:
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0o700); err != nil {
			return "", noop, fmt.Errorf("cannot create repo cache directory: %w", err)
		}
		cfg.Info("Cloning %s...\n", arg)
		if err := runGit(cfg, "clone", "--", arg, cloneDir); err != nil {
			os.RemoveAll(cloneDir)
			return "", noop, fmt.Errorf("cannot clone repository: %w", err)
		}
		return cloneDir, noop, nil
	}
}

func isGitURL(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "git@")
}

var unsafeRepoChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func repoCacheName(url string) string {
	name := strings.Trim(unsafeRepoChars.ReplaceAllString(url, "_"), "_.")
	if name == "" {
		name = "repo"
	}
	return name
}

func runGit(cfg OutputConfig, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = cfg.InfoWriter()
	cmd.Stderr = cfg.errw()
	return cmd.Run()
}

// deployRequest is shared by install and apply.
type deployRequest struct {
	cfg           OutputConfig
	stateDir      string
	repoRoot      string
	profileFlag   string
	mode          deploy.Mode
	dryRun        bool
	hooks         bool
	packages      bool
	noSecrets     bool
	prune         bool
	noInteractive bool
	command       string
}

// installReport is the JSON output of install and apply.
type installReport struct {
	Command  string            `json:"command"`
	DryRun   bool              `json:"dry_run"`
	Repo     string            `json:"repository"`
	Profile  string            `json:"profile,omitempty"`
	Detected map[string]string `json:"detected"`
	Layers   []string          `json:"layers"`
	Missing  []string          `json:"missing_layers,omitempty"`
	Actions  []installAction   `json:"actions"`
	Packages map[string]int    `json:"packages,omitempty"`
	Summary  map[string]int    `json:"summary"`
	Errors   []string          `json:"errors,omitempty"`
}

type installAction struct {
	Path      string   `json:"path"`
	Action    string   `json:"action"`
	Layer     string   `json:"layer,omitempty"`
	Kind      string   `json:"kind"`
	Reason    string   `json:"reason,omitempty"`
	Overrides []string `json:"overrides,omitempty"`
}

func runDeploy(req deployRequest) error {
	cfg := req.cfg
	home, err := homeDir()
	if err != nil {
		return err
	}

	sel, err := selectLayers(req.repoRoot, req.profileFlag)
	if err != nil {
		return err
	}
	for _, w := range sel.Warnings {
		cfg.Warn("%s", w)
	}
	detected := profileFields(sel)
	cfg.Info("Detected:\n")
	for _, k := range profileFieldOrder {
		cfg.Info("  %-9s %s\n", k+":", nonEmpty(detected[k], "-"))
	}
	if sel.ProfileName != "" {
		cfg.Info("  profile:  %s\n", sel.ProfileName)
	}

	result := sel.Result
	if len(result.Layers) == 0 {
		return nothingToDo("nothing to do: no layers found for the detected environment")
	}
	cfg.Info("\nActive layers (%d):\n", len(result.Layers))
	for _, l := range result.Layers {
		cfg.Info("  - %s\n", l.Path())
	}
	if len(result.Missing) > 0 {
		cfg.Info("Layers not in the repository: %s\n", strings.Join(missingPaths(result), ", "))
	}

	layerDirs := make([]string, len(result.Layers))
	for i, l := range result.Layers {
		layerDirs[i] = l.DirPath
	}

	sm, err := state.NewManager(req.stateDir)
	if err != nil {
		return err
	}
	bm, err := backup.NewManager(filepath.Join(req.stateDir, "backups"))
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:  req.repoRoot,
		HomeDir:   home,
		StateDir:  req.stateDir,
		Layers:    result.Layers,
		Mode:      req.mode,
		NoSecrets: req.noSecrets,
		Prune:     req.prune,
		Log:       cfg.InfoWriter(),
	}

	report := installReport{
		Command:  req.command,
		DryRun:   req.dryRun,
		Repo:     req.repoRoot,
		Profile:  sel.ProfileName,
		Detected: detected,
		Layers:   getLayerPaths(result.Layers),
		Missing:  missingPaths(result),
		Actions:  []installAction{},
		Summary:  map[string]int{},
	}

	// Hooks only run for install; apply only deploys files.
	var hooks []hook.Hook
	if req.command == "install" {
		hooks = hook.DiscoverHooks(layerDirs)
	}
	hookOpts := hook.Options{
		RepoRoot: req.repoRoot,
		HomeDir:  home,
		Profile:  sel.ProfileName,
		LogFile:  filepath.Join(req.stateDir, "hooks.log"),
	}
	if len(hooks) > 0 && !req.hooks {
		cfg.Info("\n%s skipped (use --allow-hooks or DOTF_ALLOW_HOOKS=1 to run them)\n", plural(len(hooks), "hook"))
		hooks = nil
	}
	var pm *pkg.Manager
	var pkgLists map[string][]string
	if req.packages {
		pm, pkgLists, err = loadPackageLists(cfg, layerDirs)
		if err != nil {
			return err
		}
		if len(pkgLists) > 0 {
			report.Packages = map[string]int{}
			for f, list := range pkgLists {
				report.Packages[f] = len(list)
			}
		}
	}

	if req.dryRun {
		plan, err := deploy.BuildPlan(opts, sm)
		if err != nil {
			return err
		}
		cfg.Info("\nPlan:\n")
		printPlan(cfg, plan)
		for _, h := range hooks {
			cfg.Info("  would run hook %s/%s\n", h.Layer, h.Type)
		}
		for f, list := range pkgLists {
			cfg.Info("  would install %s with %s from %s\n", plural(len(list), "package"), pm.Name(), f)
		}
		for _, it := range plan.Items {
			report.Actions = append(report.Actions, actionOf(it))
			report.Summary[string(it.Action)]++
		}
		if cfg.JSON {
			cfg.PrintJSON(report)
		}
		cfg.Info("\nDry run complete. Nothing was changed. Run 'dotf %s' without --dry-run to apply.\n", req.command)
		if req.noInteractive && len(plan.Conflicts()) > 0 {
			return conflictErr("conflict: %s would be replaced (existing files not managed by dotf)", plural(len(plan.Conflicts()), "file"))
		}
		return nil
	}

	sess, err := startSession(req.stateDir, cfg)
	if err != nil {
		return err
	}
	defer sess.end()
	opts.Abort = sess.abort

	// Re-read the state: recovering an interrupted run may have changed it.
	if sm, err = state.NewManager(req.stateDir); err != nil {
		return err
	}
	plan, err := deploy.BuildPlan(opts, sm)
	if err != nil {
		return err
	}
	if req.noInteractive {
		if c := plan.Conflicts(); len(c) > 0 {
			for _, it := range c {
				cfg.PrintErr("  conflict: %s already exists\n", it.Rel)
			}
			return conflictErr("conflict: %s already exist (aborting due to --no-interactive)", plural(len(c), "file"))
		}
	}

	if _, err := hook.ExecuteAll(hooks, hook.PreInstall, hookOpts); err != nil {
		return fmt.Errorf("%w; nothing was installed", err)
	}

	for f, list := range pkgLists {
		cfg.Info("\nInstalling %s with %s from %s...\n", plural(len(list), "package"), pm.Name(), f)
		if err := pm.Install(list); err != nil {
			return fmt.Errorf("package installation from %s failed: %w", f, err)
		}
	}

	if len(plan.Items) > 0 {
		cfg.Info("\n")
	}
	res, err := deploy.Execute(opts, plan, sm, bm)
	if err != nil {
		hook.ExecuteAll(hooks, hook.OnError, hookOpts)
		return err
	}

	sm.SetRepository(req.repoRoot)
	sm.RecordInstall(getLayerPaths(result.Layers))
	if err := sm.Save(); err != nil {
		return fmt.Errorf("files installed but the state could not be saved: %w", err)
	}

	_, hookErr := hook.ExecuteAll(hooks, hook.PostInstall, hookOpts)

	for _, it := range plan.Items {
		report.Actions = append(report.Actions, actionOf(it))
	}
	report.Summary = map[string]int{
		"created":   res.Created,
		"replaced":  res.Replaced,
		"updated":   res.Updated,
		"unchanged": res.Unchanged,
		"removed":   res.Removed,
		"backed_up": res.BackedUp,
		"errors":    len(res.Errors),
	}
	report.Errors = res.Errors
	if cfg.JSON {
		cfg.PrintJSON(report)
	}

	summary := joinNonEmpty([]string{
		countPart(res.Created, "created"),
		countPart(res.Replaced, "replaced"),
		countPart(res.Updated, "updated"),
		countPart(res.Removed, "removed"),
		countPart(res.BackedUp, "backed up"),
		countPart(res.Unchanged, "unchanged"),
	}, ", ")
	cfg.Info("\nDone: %s\n", nonEmpty(summary, "nothing to change"))

	if len(res.Errors) > 0 {
		for _, e := range res.Errors {
			cfg.PrintErr("  error: %s\n", e)
		}
		return fmt.Errorf("%s could not be installed", plural(len(res.Errors), "file"))
	}
	return hookErr
}

var profileFieldOrder = []string{"distro", "session", "wm", "desktop", "shell", "terminal", "gpu", "hostname", "device"}

func profileFields(sel *selection) map[string]string {
	p := sel.Profile
	return map[string]string{
		"distro":   p.Distro,
		"session":  p.Session,
		"wm":       p.WM,
		"desktop":  p.DE,
		"shell":    p.Shell,
		"terminal": p.Terminal,
		"gpu":      p.GPU,
		"hostname": p.Hostname,
		"device":   p.DeviceType,
	}
}

func countPart(n int, word string) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%d %s", n, word)
}

func missingPaths(r *layer.ResolveResult) []string {
	var out []string
	for _, l := range r.Missing {
		out = append(out, l.Path())
	}
	return out
}

func actionOf(it deploy.Item) installAction {
	return installAction{
		Path:      it.Rel,
		Action:    string(it.Action),
		Layer:     it.Layer,
		Kind:      string(it.Kind),
		Reason:    it.Reason,
		Overrides: it.Overridden,
	}
}

func printPlan(cfg OutputConfig, plan *deploy.Plan) {
	for _, it := range plan.Items {
		if it.Action == deploy.ActUnchanged {
			continue
		}
		line := fmt.Sprintf("  %-9s %s", it.Action, it.Rel)
		if it.Layer != "" && it.Action != deploy.ActRemove {
			line += " <- " + it.Layer
		}
		if it.Kind == deploy.KindSecret {
			line += " (secret)"
		}
		if len(it.Overridden) > 0 {
			line += " (overrides " + strings.Join(it.Overridden, ", ") + ")"
		}
		if it.Action == deploy.ActReplace {
			line += " (existing file will be backed up)"
		}
		if it.Reason != "" {
			line += ": " + it.Reason
		}
		cfg.Info("%s\n", line)
	}
	if n := plan.Count(deploy.ActUnchanged); n > 0 {
		cfg.Info("  %s already up to date\n", plural(n, "file"))
	}
	if len(plan.Items) == 0 {
		cfg.Info("  (no files)\n")
	}
}

// loadPackageLists reads the package lists meant for the detected manager.
func loadPackageLists(cfg OutputConfig, layerDirs []string) (*pkg.Manager, map[string][]string, error) {
	pm := pkg.DetectManager()
	if pm == nil {
		return nil, nil, nil
	}
	used, ignored := pm.ListFiles(layerDirs)
	for _, f := range ignored {
		cfg.Info("Ignoring %s (not a package list for %s)\n", f, pm.Name())
	}
	lists := map[string][]string{}
	for _, f := range used {
		pkgs, err := pkg.LoadPackages(f)
		if err != nil {
			return nil, nil, err
		}
		if len(pkgs) > 0 {
			lists[f] = pkgs
		}
	}
	return pm, lists, nil
}

// getLayerPaths extracts layer path strings from resolved layers.
func getLayerPaths(layers []layer.ResolvedLayer) []string {
	paths := make([]string, len(layers))
	for i, l := range layers {
		paths[i] = l.Path()
	}
	return paths
}
