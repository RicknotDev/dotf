package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// OutputConfig controls CLI output behavior.
type OutputConfig struct {
	JSON    bool
	Quiet   bool
	NoColor bool
	Filter  string // filter string for status, e.g., "profile:base" or "state:ok"
}

// ParseGlobalFlags parses global flags from args and returns remaining args.
func ParseGlobalFlags(args []string) (OutputConfig, []string) {
	cfg := OutputConfig{}
	remaining := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			cfg.JSON = true
		case "--quiet":
			cfg.Quiet = true
		case "--no-color":
			cfg.NoColor = true
		case "--filter":
			if i+1 < len(args) {
				i++
				cfg.Filter = args[i]
			}
		default:
			remaining = append(remaining, args[i])
		}
	}

	// Respect NO_COLOR environment variable
	if os.Getenv("NO_COLOR") != "" {
		cfg.NoColor = true
	}

	return cfg, remaining
}

// Print prints a message to stdout respecting the output config.
func (c OutputConfig) Print(msg string) {
	if c.Quiet {
		return
	}
	if c.JSON {
		// In JSON mode, stdout is reserved for JSON output
		return
	}
	fmt.Println(msg)
}

// Printf formats and prints to stdout respecting the output config.
func (c OutputConfig) Printf(format string, args ...interface{}) {
	if c.Quiet {
		return
	}
	if c.JSON {
		return
	}
	fmt.Printf(format, args...)
}

// PrintErr prints an error message to stderr respecting quiet mode.
func (c OutputConfig) PrintErr(format string, args ...interface{}) {
	if c.Quiet && !c.JSON {
		return
	}
	msg := fmt.Sprintf(format, args...)
	if c.JSON {
		// In JSON mode, errors are output as JSON on stderr
		errOutput := map[string]interface{}{
			"error": msg,
			"level": "error",
		}
		b, _ := json.Marshal(errOutput)
		fmt.Fprintln(os.Stderr, string(b))
		return
	}
	fmt.Fprint(os.Stderr, msg)
}

// PrintErrln prints an error line to stderr.
func (c OutputConfig) PrintErrln(msg string) {
	c.PrintErr("%s\n", msg)
}

// PrintJSON prints a JSON representation to stdout.
func (c OutputConfig) PrintJSON(v interface{}) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		c.PrintErr("json error: %v\n", err)
		return
	}
	fmt.Println(string(b))
}

// Colorize wraps text in color codes if color is enabled.
func (c OutputConfig) Colorize(colorCode, text string) string {
	if c.NoColor {
		return text
	}
	return fmt.Sprintf("\033[%sm%s\033[0m", colorCode, text)
}

// Red returns red text if color is enabled.
func (c OutputConfig) Red(text string) string {
	return c.Colorize("31", text)
}

// Green returns green text if color is enabled.
func (c OutputConfig) Green(text string) string {
	return c.Colorize("32", text)
}

// Yellow returns yellow text if color is enabled.
func (c OutputConfig) Yellow(text string) string {
	return c.Colorize("33", text)
}

// Bold returns bold text if color is enabled.
func (c OutputConfig) Bold(text string) string {
	return c.Colorize("1", text)
}

// Dim returns dim text if color is enabled.
func (c OutputConfig) Dim(text string) string {
	return c.Colorize("2", text)
}

// matchesFilter checks if a string matches the configured filter.
// Supports "key:value" format and simple substring match.
func (c OutputConfig) matchesFilter(s string) bool {
	if c.Filter == "" {
		return true
	}

	parts := strings.SplitN(c.Filter, ":", 2)
	if len(parts) == 2 {
		// Structured filter: "field:value"
		return strings.Contains(strings.ToLower(s), strings.ToLower(parts[1]))
	}
	// Simple substring filter
	return strings.Contains(strings.ToLower(s), strings.ToLower(c.Filter))
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
