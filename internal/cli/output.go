package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// OutputConfig controls CLI output behavior.
//
// stdout carries results (or JSON with --json). stderr carries progress
// (Info, hidden by --quiet and --json) and errors (always shown; as JSON
// objects with --json).
type OutputConfig struct {
	JSON    bool
	Quiet   bool
	NoColor bool
	Filter  string // "value" or "key:value", e.g. "broken" or "layer:base"

	Stdout io.Writer // defaults to os.Stdout
	Stderr io.Writer // defaults to os.Stderr
}

// ParseGlobalFlags extracts the global flags from anywhere in args and
// returns the remaining arguments in order. Accepts --filter value and
// --filter=value.
func ParseGlobalFlags(args []string) (OutputConfig, []string) {
	cfg := OutputConfig{}
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			remaining = append(remaining, args[i:]...)
			i = len(args)
		case a == "--json":
			cfg.JSON = true
		case a == "--quiet" || a == "-q":
			cfg.Quiet = true
		case a == "--no-color":
			cfg.NoColor = true
		case a == "--filter":
			if i+1 < len(args) {
				i++
				cfg.Filter = args[i]
			}
		case strings.HasPrefix(a, "--filter="):
			cfg.Filter = strings.TrimPrefix(a, "--filter=")
		default:
			remaining = append(remaining, a)
		}
	}
	if os.Getenv("NO_COLOR") != "" {
		cfg.NoColor = true
	}
	return cfg, remaining
}

// IsTerminal reports whether f is a character device (a terminal).
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (c OutputConfig) out() io.Writer {
	if c.Stdout != nil {
		return c.Stdout
	}
	return os.Stdout
}

func (c OutputConfig) errw() io.Writer {
	if c.Stderr != nil {
		return c.Stderr
	}
	return os.Stderr
}

// Print prints a line of human-readable output to stdout (suppressed by
// --quiet and --json).
func (c OutputConfig) Print(msg string) {
	if c.Quiet || c.JSON {
		return
	}
	fmt.Fprintln(c.out(), msg)
}

// Printf formats human-readable output to stdout (suppressed by --quiet
// and --json).
func (c OutputConfig) Printf(format string, args ...interface{}) {
	if c.Quiet || c.JSON {
		return
	}
	fmt.Fprintf(c.out(), format, args...)
}

// Info prints progress to stderr (suppressed by --quiet and --json).
func (c OutputConfig) Info(format string, args ...interface{}) {
	if c.Quiet || c.JSON {
		return
	}
	fmt.Fprintf(c.errw(), format, args...)
}

// InfoWriter returns the progress writer (io.Discard under --quiet/--json).
func (c OutputConfig) InfoWriter() io.Writer {
	if c.Quiet || c.JSON {
		return io.Discard
	}
	return c.errw()
}

// Warn prints a warning to stderr. Warnings are shown even with --quiet.
func (c OutputConfig) Warn(format string, args ...interface{}) {
	msg := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	if c.JSON {
		b, _ := json.Marshal(map[string]string{"level": "warning", "message": msg})
		fmt.Fprintln(c.errw(), string(b))
		return
	}
	fmt.Fprintln(c.errw(), "warning: "+msg)
}

// PrintErr prints an error message to stderr. Errors are never hidden by
// --quiet; with --json they are printed as {"level":"error","error":...}.
func (c OutputConfig) PrintErr(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if c.JSON {
		b, _ := json.Marshal(map[string]interface{}{
			"error": strings.TrimRight(msg, "\n"),
			"level": "error",
		})
		fmt.Fprintln(c.errw(), string(b))
		return
	}
	fmt.Fprint(c.errw(), msg)
}

// PrintErrln prints an error line to stderr.
func (c OutputConfig) PrintErrln(msg string) {
	c.PrintErr("%s\n", msg)
}

// PrintJSON prints v as indented JSON to stdout.
func (c OutputConfig) PrintJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		c.PrintErr("json error: %v\n", err)
		return
	}
	fmt.Fprintln(c.out(), string(b))
}

// Colorize wraps text in color codes if color is enabled.
func (c OutputConfig) Colorize(colorCode, text string) string {
	if c.NoColor {
		return text
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", colorCode, text)
}

// Red returns red text if color is enabled.
func (c OutputConfig) Red(text string) string { return c.Colorize("31", text) }

// Green returns green text if color is enabled.
func (c OutputConfig) Green(text string) string { return c.Colorize("32", text) }

// Yellow returns yellow text if color is enabled.
func (c OutputConfig) Yellow(text string) string { return c.Colorize("33", text) }

// Bold returns bold text if color is enabled.
func (c OutputConfig) Bold(text string) string { return c.Colorize("1", text) }

// Dim returns dim text if color is enabled.
func (c OutputConfig) Dim(text string) string { return c.Colorize("2", text) }

// matchesFilter matches a single string against the filter value
// (case-insensitive substring; for "key:value" only the value is used).
func (c OutputConfig) matchesFilter(s string) bool {
	if c.Filter == "" {
		return true
	}
	value := c.Filter
	if parts := strings.SplitN(c.Filter, ":", 2); len(parts) == 2 {
		value = parts[1]
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(value))
}

// matchFields matches named fields against the filter. "key:value" only
// looks at that field (error for an unknown key); a bare value matches any
// field.
func (c OutputConfig) matchFields(fields map[string]string) (bool, error) {
	if c.Filter == "" {
		return true, nil
	}
	if parts := strings.SplitN(c.Filter, ":", 2); len(parts) == 2 {
		key := strings.ToLower(parts[0])
		v, ok := fields[key]
		if !ok {
			keys := make([]string, 0, len(fields))
			for k := range fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return false, &ExitError{Code: ExitUsage, Err: fmt.Errorf("unknown filter key %q (use one of: %s)", key, strings.Join(keys, ", "))}
		}
		return strings.Contains(strings.ToLower(v), strings.ToLower(parts[1])), nil
	}
	for _, v := range fields {
		if strings.Contains(strings.ToLower(v), strings.ToLower(c.Filter)) {
			return true, nil
		}
	}
	return false, nil
}

// Exit codes.
const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitConflict    = 2 // unresolved conflict (apply --no-interactive)
	ExitNothingToDo = 3
	ExitUsage       = 64
)

// ExitError carries the process exit code for an error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode returns the exit code for err (0 for nil, 1 by default).
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return ExitFailure
}

func conflictErr(format string, args ...interface{}) error {
	return &ExitError{Code: ExitConflict, Err: fmt.Errorf(format, args...)}
}

func nothingToDo(format string, args ...interface{}) error {
	return &ExitError{Code: ExitNothingToDo, Err: fmt.Errorf(format, args...)}
}

func usageErr(format string, args ...interface{}) error {
	return &ExitError{Code: ExitUsage, Err: fmt.Errorf(format, args...)}
}

// FilterStatus represents the status of an installed file or operation.
type FilterStatus string

const (
	StatusOK       FilterStatus = "ok"
	StatusBroken   FilterStatus = "broken"
	StatusMissing  FilterStatus = "missing"
	StatusConflict FilterStatus = "conflict"
	StatusSkipped  FilterStatus = "skipped"
	StatusError    FilterStatus = "error"
)

// StatusEntry represents a single status entry for JSON output.
type StatusEntry struct {
	Path   string       `json:"path"`
	Status FilterStatus `json:"status"`
	Type   string       `json:"type,omitempty"`
	Detail string       `json:"detail,omitempty"`
	Layer  string       `json:"layer,omitempty"`
	Target string       `json:"target,omitempty"`
}

// StatusResult represents the full status result for JSON output.
type StatusResult struct {
	Version int           `json:"version"`
	Entries []StatusEntry `json:"entries"`
	Summary StatusSummary `json:"summary"`
}

// StatusSummary summarizes the status check.
type StatusSummary struct {
	Total   int `json:"total"`
	OK      int `json:"ok"`
	Issues  int `json:"issues"`
	Broken  int `json:"broken"`
	Missing int `json:"missing"`
}
