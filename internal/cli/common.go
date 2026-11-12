package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/RicknotDev/dotf/internal/lock"
	"github.com/RicknotDev/dotf/internal/state"
	"github.com/RicknotDev/dotf/internal/transaction"
)

// LockTimeout is how long commands wait for another dotf process.
var LockTimeout = 30 * time.Second

// newFlagSet returns a flag set that reports errors as usage errors instead
// of printing the whole flag list.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return usageErr("%s: %v (see 'dotf %s --help')", fs.Name(), err, fs.Name())
	}
	return nil
}

// isRepo reports whether dir looks like a DOTF repository.
func isRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "layers"))
	return err == nil && info.IsDir()
}

// findRepo returns the repository to work on: the current directory if it
// is a DOTF repository, otherwise the repository recorded by the last
// install. ok is false when neither is usable; repo is then the current
// directory.
func findRepo(stateDir string) (repo string, ok bool, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("cannot determine working directory: %w", err)
	}
	if isRepo(cwd) {
		return cwd, true, nil
	}
	if sm, serr := state.NewManager(stateDir); serr == nil {
		if r := sm.GetState().Repository; r != "" && isRepo(r) {
			return r, true, nil
		}
	}
	return cwd, false, nil
}

func requireRepo(stateDir string) (string, error) {
	repo, ok, err := findRepo(stateDir)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("not a DOTF repository: no 'layers' directory in %s, and no repository recorded by a previous install", repo)
	}
	return repo, nil
}

// session holds the lock of a mutating command and recovers interrupted
// transactions before the command starts.
type session struct {
	lock  *lock.Lock
	abort chan struct{}
	stop  func()
}

func startSession(stateDir string, cfg OutputConfig) (*session, error) {
	l, err := lock.Acquire(stateDir, LockTimeout)
	if err != nil {
		return nil, fmt.Errorf("cannot acquire lock: %w", err)
	}
	n, err := transaction.Recover(stateDir, cfg.InfoWriter())
	if err != nil {
		l.Release()
		return nil, fmt.Errorf("cannot recover an interrupted transaction (run 'dotf doctor --fix'): %w", err)
	}
	if n > 0 {
		cfg.Warn("recovered %d interrupted transaction(s) from a previous run", n)
	}

	s := &session{lock: l, abort: make(chan struct{})}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case sig := <-sigCh:
			cfg.PrintErr("\nReceived %v, stopping and rolling back...\n", sig)
			close(s.abort)
		case <-done:
		}
	}()
	s.stop = func() {
		signal.Stop(sigCh)
		close(done)
		l.Release()
	}
	return s, nil
}

func (s *session) end() {
	if s != nil && s.stop != nil {
		s.stop()
		s.stop = nil
	}
}

func homeDir() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return "", fmt.Errorf("cannot determine home directory: $HOME is not set")
	}
	return filepath.Clean(h), nil
}

// nonEmpty returns the string value or a fallback if empty.
func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// isSymlink checks if a path is a symbolic link.
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// plural returns "1 file" / "2 files".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func joinNonEmpty(parts []string, sep string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
