package detect

import (
	"os"
	"strings"
)

// detectTerminal identifies the terminal emulator.
// Priority: TERMINAL env > TERM_PROGRAM env > parent process inspection > TERM env.
func detectTerminal() string {
	// 1. Direct env var (some terminals set this)
	if t := os.Getenv("TERMINAL"); t != "" {
		return normalizeTerminal(t)
	}

	// 2. TERM_PROGRAM (iTerm2, tmux, etc.)
	if t := os.Getenv("TERM_PROGRAM"); t != "" {
		return normalizeTerminal(t)
	}

	// 3. Inspect parent process chain
	if t := detectTerminalByProcess(); t != "" {
		return t
	}

	// 4. Last resort: check TERM (usually "xterm-256color" but sometimes specific)
	if t := os.Getenv("TERM"); t != "" && t != "xterm" && t != "xterm-256color" && t != "screen" && t != "screen-256color" {
		return normalizeTerminal(t)
	}

	return ""
}

// knownTerminals maps process names to canonical terminal names.
var knownTerminals = map[string]string{
	"wezterm":         "wezterm",
	"kitty":           "kitty",
	"foot":            "foot",
	"ghostty":         "ghostty",
	"alacritty":       "alacritty",
	"alacritty-msg":   "alacritty",
	"konsole":         "konsole",
	"gnome-terminal":  "gnome-terminal",
	"gnome-terminal-": "gnome-terminal",
	"xfce4-terminal":  "xfce4-terminal",
	"lxterminal":      "lxterminal",
	"urxvt":           "rxvt-unicode",
	"urxvtd":          "rxvt-unicode",
	"st":              "st",
	"st-256color":     "st",
	"xterm":           "xterm",
	"terminator":      "terminator",
	"termite":         "termite",
	"tilix":           "tilix",
	"cool-retro-term": "cool-retro-term",
	"contour":         "contour",
	"rio":             "rio",
	"warp":            "warp",
	"tabby":           "tabby",
	"hyper":           "hyper",
	"windowsterminal": "windows-terminal",
	"blackbox":        "blackbox",
	"ptyxis":          "ptyxis",
}

// detectTerminalByProcess walks up the process tree to find a known terminal.
func detectTerminalByProcess() string {
	// Walk up a few levels of parent processes
	pid := os.Getppid()
	for i := 0; i < 10 && pid > 1; i++ {
		comm, err := os.ReadFile("/proc/" + itoa(pid) + "/comm")
		if err != nil {
			break
		}
		name := strings.TrimSpace(string(comm))
		if term, ok := knownTerminals[strings.ToLower(name)]; ok {
			return term
		}
		// Try to get parent PID
		status, err := os.ReadFile("/proc/" + itoa(pid) + "/status")
		if err != nil {
			break
		}
		pid = parsePPid(string(status))
	}
	return ""
}

// parsePPid extracts the PPid field from /proc/pid/status.
func parsePPid(status string) int {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "PPid:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "PPid:"))
			var ppid int
			for _, c := range val {
				if c < '0' || c > '9' {
					break
				}
				ppid = ppid*10 + int(c-'0')
			}
			return ppid
		}
	}
	return 1
}

// itoa is a simple int-to-string for positive ints (avoids strconv import).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// normalizeTerminal lowercases and strips suffixes.
func normalizeTerminal(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".exe")
	if term, ok := knownTerminals[s]; ok {
		return term
	}
	// If unknown, return the raw name for extensibility
	return s
}
