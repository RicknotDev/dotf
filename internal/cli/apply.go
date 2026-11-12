package cli

import (
	"fmt"

	"github.com/RicknotDev/dotf/internal/deploy"
)

const applyHelp = `Usage: dotf apply [options]

Deploy the resolved layers' files into your home directory. Unlike install,
apply never runs hooks or installs packages, and does not clone repositories.
It uses the current directory, or the repository of the last install.

Options:
  --profile <name>     Use a profile from dotf.yaml
  --dry-run            Show what would be done without changing anything
  --diff               Same as --dry-run
  --copy               Copy files instead of creating symlinks
  --no-secrets         Do not deploy secrets
  --no-interactive     Abort (exit code 2) instead of replacing files that
                       dotf does not manage
  --json               Output in JSON format (global flag)
  --help               Show this help message

Examples:
  dotf apply                    # Apply auto-detected layers
  dotf apply --profile laptop   # Apply a profile from dotf.yaml
  dotf apply --dry-run          # Preview changes
`

// Apply runs the apply command.
func Apply(args []string, stateDir string, cfg OutputConfig) error {
	fs := newFlagSet("apply")
	profileName := fs.String("profile", "", "")
	dryRun := fs.Bool("dry-run", false, "")
	diff := fs.Bool("diff", false, "")
	copyMode := fs.Bool("copy", false, "")
	noSecrets := fs.Bool("no-secrets", false, "")
	noInteractive := fs.Bool("no-interactive", false, "")
	help := fs.Bool("help", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *help {
		fmt.Fprint(cfg.out(), applyHelp)
		return nil
	}
	if fs.NArg() > 0 {
		return usageErr("apply takes no arguments (got %q)", fs.Arg(0))
	}

	repoRoot, err := requireRepo(stateDir)
	if err != nil {
		return err
	}
	mode := deploy.Symlink
	if *copyMode {
		mode = deploy.Copy
	}
	return runDeploy(deployRequest{
		cfg:           cfg,
		stateDir:      stateDir,
		repoRoot:      repoRoot,
		profileFlag:   *profileName,
		mode:          mode,
		dryRun:        *dryRun || *diff,
		noSecrets:     *noSecrets,
		prune:         true,
		noInteractive: *noInteractive,
		command:       "apply",
	})
}
