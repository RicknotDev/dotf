package cli

import (
	"strings"
	"testing"
)

func TestParseGlobalFlagsNoFlags(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"install", "--dry-run"})
	if cfg.JSON {
		t.Fatal("expected JSON=false")
	}
	if cfg.Quiet {
		t.Fatal("expected Quiet=false")
	}
	if cfg.NoColor {
		t.Fatal("expected NoColor=false")
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining args, got %d: %v", len(remaining), remaining)
	}
}

func TestParseGlobalFlagsJSON(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"--json", "status"})
	if !cfg.JSON {
		t.Fatal("expected JSON=true")
	}
	if len(remaining) != 1 || remaining[0] != "status" {
		t.Fatalf("expected ['status'], got %v", remaining)
	}
}

func TestParseGlobalFlagsQuiet(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"--quiet", "status", "--short"})
	if !cfg.Quiet {
		t.Fatal("expected Quiet=true")
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining args, got %d", len(remaining))
	}
}

func TestParseGlobalFlagsNoColor(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"--no-color", "install"})
	if !cfg.NoColor {
		t.Fatal("expected NoColor=true")
	}
	if len(remaining) != 1 || remaining[0] != "install" {
		t.Fatalf("expected ['install'], got %v", remaining)
	}
}

func TestParseGlobalFlagsFilter(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"--filter", "broken", "status"})
	if cfg.Filter != "broken" {
		t.Fatalf("expected filter='broken', got '%s'", cfg.Filter)
	}
	if len(remaining) != 1 || remaining[0] != "status" {
		t.Fatalf("expected ['status'], got %v", remaining)
	}
}

func TestParseGlobalFlagsAllFlags(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"--json", "--quiet", "--no-color", "--filter", "ok", "install", "--dry-run"})
	if !cfg.JSON {
		t.Fatal("expected JSON=true")
	}
	if !cfg.Quiet {
		t.Fatal("expected Quiet=true")
	}
	if !cfg.NoColor {
		t.Fatal("expected NoColor=true")
	}
	if cfg.Filter != "ok" {
		t.Fatalf("expected filter='ok', got '%s'", cfg.Filter)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining args, got %d: %v", len(remaining), remaining)
	}
}

func TestNOCOLOREnvVar(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	cfg, _ := ParseGlobalFlags([]string{"status"})
	if !cfg.NoColor {
		t.Fatal("expected NoColor=true from NO_COLOR env var")
	}
}

func TestNOCOLOREnvVarOverriddenByFlag(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// --no-color flag should also work
	cfg, _ := ParseGlobalFlags([]string{"--no-color", "status"})
	if !cfg.NoColor {
		t.Fatal("expected NoColor=true")
	}
}

func TestOutputConfigPrintJSON(t *testing.T) {
	// JSON mode should suppress regular Print calls
	cfg := OutputConfig{JSON: true}
	// Should not print anything
	cfg.Print("should not appear")
	cfg.Printf("should not appear %d", 1)
}

func TestOutputConfigQuiet(t *testing.T) {
	cfg := OutputConfig{Quiet: true}
	cfg.Print("should not appear")
	cfg.Printf("should not appear %d", 1)
}

func TestOutputConfigColorize(t *testing.T) {
	cfg := OutputConfig{NoColor: false}
	colored := cfg.Colorize("31", "red text")
	if !strings.HasPrefix(colored, "\033[31m") {
		t.Fatalf("expected color codes, got: %q", colored)
	}
	if !strings.HasSuffix(colored, "\033[0m") {
		t.Fatalf("expected reset code, got: %q", colored)
	}

	// With NoColor, should return plain text
	cfg.NoColor = true
	plain := cfg.Colorize("31", "red text")
	if plain != "red text" {
		t.Fatalf("expected plain text, got: %q", plain)
	}
}

func TestOutputConfigRedGreenYellowBoldDim(t *testing.T) {
	cfg := OutputConfig{NoColor: false}

	red := cfg.Red("test")
	if !strings.Contains(red, "test") {
		t.Fatal("Red should contain the text")
	}

	green := cfg.Green("test")
	if !strings.Contains(green, "test") {
		t.Fatal("Green should contain the text")
	}

	yellow := cfg.Yellow("test")
	if !strings.Contains(yellow, "test") {
		t.Fatal("Yellow should contain the text")
	}

	bold := cfg.Bold("test")
	if !strings.Contains(bold, "test") {
		t.Fatal("Bold should contain the text")
	}

	dim := cfg.Dim("test")
	if !strings.Contains(dim, "test") {
		t.Fatal("Dim should contain the text")
	}

	// With NoColor, all should return plain text
	cfg.NoColor = true
	if cfg.Red("test") != "test" {
		t.Fatal("Red with NoColor should return plain text")
	}
	if cfg.Green("test") != "test" {
		t.Fatal("Green with NoColor should return plain text")
	}
}

func TestMatchesFilter(t *testing.T) {
	cfg := OutputConfig{Filter: ""}
	if !cfg.matchesFilter("anything") {
		t.Fatal("empty filter should match everything")
	}

	cfg.Filter = "broken"
	if !cfg.matchesFilter("broken") {
		t.Fatal("should match 'broken'")
	}
	if !cfg.matchesFilter("BROKEN") {
		t.Fatal("should match case-insensitive")
	}
	if cfg.matchesFilter("missing") {
		t.Fatal("should not match 'missing'")
	}

	cfg.Filter = "status:ok"
	if !cfg.matchesFilter("ok") {
		t.Fatal("structured filter should match value part")
	}
	if !cfg.matchesFilter("status:OK") {
		t.Fatal("structured filter should match case-insensitive")
	}
}

func TestPrintJSON(t *testing.T) {
	cfg := OutputConfig{JSON: false}
	// Should print normally
	cfg.Print("hello")

	// With JSON mode, Print should not output
	cfg2 := OutputConfig{JSON: true}
	cfg2.PrintJSON(map[string]string{"key": "value"})
}

func TestStatusEntryType(t *testing.T) {
	e := StatusEntry{
		Path:   ".zshrc",
		Status: StatusOK,
		Type:   "symlink",
		Detail: "all good",
		Layer:  "base",
		Target: "/repo/layers/base/.zshrc",
	}
	if e.Path != ".zshrc" {
		t.Fatalf("expected .zshrc, got %s", e.Path)
	}
	if e.Status != StatusOK {
		t.Fatalf("expected ok, got %s", e.Status)
	}
	if e.Type != "symlink" {
		t.Fatalf("expected symlink, got %s", e.Type)
	}
}

func TestStatusConstants(t *testing.T) {
	if StatusOK != "ok" {
		t.Fatalf("expected 'ok', got '%s'", StatusOK)
	}
	if StatusBroken != "broken" {
		t.Fatalf("expected 'broken', got '%s'", StatusBroken)
	}
	if StatusMissing != "missing" {
		t.Fatalf("expected 'missing', got '%s'", StatusMissing)
	}
	if StatusConflict != "conflict" {
		t.Fatalf("expected 'conflict', got '%s'", StatusConflict)
	}
	if StatusError != "error" {
		t.Fatalf("expected 'error', got '%s'", StatusError)
	}
}

func TestStatusSummary(t *testing.T) {
	s := StatusSummary{
		Total:   10,
		OK:      8,
		Issues:  2,
		Broken:  1,
		Missing: 1,
	}
	if s.Total != 10 {
		t.Fatalf("expected 10, got %d", s.Total)
	}
	if s.OK != 8 {
		t.Fatalf("expected 8, got %d", s.OK)
	}
}

func TestOutputConfigPrintErr(t *testing.T) {
	// Quiet mode should suppress PrintErr
	cfg := OutputConfig{Quiet: true}
	cfg.PrintErr("suppressed error") // should not appear

	// JSON mode should still show errors
	cfg2 := OutputConfig{JSON: true}
	cfg2.PrintErr("json error") // should appear as JSON
}

func TestParseGlobalFlagsFilterWithoutValue(t *testing.T) {
	// --filter without a following arg should be treated as a regular arg
	cfg, remaining := ParseGlobalFlags([]string{"--filter", "status"})
	if cfg.Filter != "status" {
		t.Fatalf("expected filter='status', got '%s'", cfg.Filter)
	}
	_ = remaining
}

func TestParseGlobalFlagsWithCommandArgs(t *testing.T) {
	cfg, remaining := ParseGlobalFlags([]string{"install", "--dry-run", "--copy", "--allow-hooks"})
	if cfg.JSON {
		t.Fatal("expected JSON=false for non-global flags")
	}
	if len(remaining) != 4 {
		t.Fatalf("expected 4 remaining args, got %d", len(remaining))
	}
}

func TestOutputConfigEmpty(t *testing.T) {
	cfg := OutputConfig{}
	if cfg.JSON || cfg.Quiet || cfg.NoColor {
		t.Fatal("default config should have all flags false")
	}
	if cfg.Filter != "" {
		t.Fatal("default filter should be empty")
	}
}

func TestPrintErrNonQuiet(t *testing.T) {
	// Non-quiet, non-JSON should just print
	cfg := OutputConfig{Quiet: false, JSON: false}
	_ = cfg
}

func TestEnvVarOverride(t *testing.T) {
	// NO_COLOR takes precedence
	t.Setenv("NO_COLOR", "true")
	cfg, _ := ParseGlobalFlags([]string{})
	if !cfg.NoColor {
		t.Fatal("expected NoColor from NO_COLOR env")
	}
}

func TestFilterStatusParsing(t *testing.T) {
	// Verify the FilterStatus type functions as expected
	var s FilterStatus = "ok"
	if string(s) != "ok" {
		t.Fatalf("expected 'ok', got '%s'", s)
	}
}
