// Package detect discovers the current Linux environment by inspecting
// /proc, /sys, and environment variables. It never hardcodes support tables;
// detection is based on runtime discovery.
package detect

import (
	"os"
	"strings"

	"github.com/codebuff/dotf/internal/profile"
)

// Detect performs full environment detection and returns a Profile.
func Detect() profile.Profile {
	return profile.Profile{
		Distro:     detectDistro(),
		Session:    detectSession(),
		WM:         detectWM(),
		DE:         detectDE(),
		Shell:      detectShell(),
		Terminal:   detectTerminal(),
		GPU:        detectGPU(),
		Hostname:   detectHostname(),
		DeviceType: detectDeviceType(),
	}
}

// detectDE extracts the desktop environment name, separate from WM detection.
// It checks XDG_CURRENT_DESKTOP and other indicators.
func detectDE() string {
	de := detectDEFromEnv()
	if de != "" {
		return de
	}
	// Fallback: if a known DE process is running
	return detectDEByProcess()
}

// detectDEFromEnv reads desktop environment variables.
func detectDEFromEnv() string {
	// XDG_CURRENT_DESKTOP often contains the DE name
	if xdg := os.Getenv("XDG_CURRENT_DESKTOP"); xdg != "" {
		xdg = strings.ToLower(xdg)
		for _, de := range knownDEs {
			if strings.Contains(xdg, de) {
				return de
			}
		}
	}

	// DESKTOP_SESSION also works
	if ds := os.Getenv("DESKTOP_SESSION"); ds != "" {
		ds = strings.ToLower(ds)
		for _, de := range knownDEs {
			if strings.Contains(ds, de) {
				return de
			}
		}
	}

	// GDMSESSION (GDM-specific)
	if gdm := os.Getenv("GDMSESSION"); gdm != "" {
		gdm = strings.ToLower(gdm)
		for _, de := range knownDEs {
			if strings.Contains(gdm, de) {
				return de
			}
		}
	}

	return ""
}

// knownDEs is a list of desktop environment identifiers for detection.
var knownDEs = []string{
	"gnome", "kde", "plasma", "xfce", "cinnamon",
	"mate", "lxqt", "lxde", "budgie", "deepin",
	"pantheon", "cosmic", "unity", "lumina",
}

// detectDEByProcess scans for known DE-specific processes.
func detectDEByProcess() string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return ""
	}

	deProcs := map[string]string{
		"gnome-shell":      "gnome",
		"plasmashell":      "kde",
		"xfce4-session":    "xfce",
		"cinnamon-session": "cinnamon",
		"mate-session":     "mate",
		"lxqt-session":     "lxqt",
		"lxpanel":          "lxde",
		"budgie-wm":        "budgie",
		"deepin-wm":        "deepin",
		"gala":             "pantheon",
		"cosmic-comp":      "cosmic",
		"unity-session":    "unity",
	}

	// Limit to first 500 PIDs to avoid performance issues on servers
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name[0] < '0' || name[0] > '9' {
			continue
		}
		count++
		if count > 500 {
			break
		}
		comm, err := os.ReadFile("/proc/" + name + "/comm")
		if err != nil {
			continue
		}
		procName := strings.TrimSpace(string(comm))
		if de, ok := deProcs[strings.ToLower(procName)]; ok {
			return de
		}
	}

	return ""
}
