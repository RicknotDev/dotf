package detect

import (
	"os"
	"strings"
)

// knownWMs is a list of window manager process names we can detect.
// DOTF never hardcodes support — this is only for *detection* from the running system.
// If a WM isn't in this list, DOTF still works; it just won't detect it by process name.
var knownWMs = map[string]string{
	"hyprland":   "hyprland",
	"qtile":      "qtile",
	"river":      "river",
	"dwm":        "dwm",
	"awesome":    "awesome",
	"i3":         "i3",
	"sway":       "sway",
	"bspwm":      "bspwm",
	"xmonad":     "xmonad",
	"herbstluft": "herbstluftwm",
	"openbox":    "openbox",
	"fluxbox":    "fluxbox",
	"icewm":      "icewm",
	"kwin":       "kwin",
	"mutter":     "mutter",
	"marco":      "marco",
	"xfwm4":      "xfwm4",
	"cinnamon":   "cinnamon",
	"budgie-wm":  "budgie-wm",
	"leftwm":     "leftwm",
	"niri":       "niri",
	"cosmic-comp":"cosmic-comp",
}

// detectWM attempts to identify the window manager / compositor.
// Priority: XDG_CURRENT_DESKTOP > DESKTOP_SESSION > process scan.
func detectWM() string {
	// 1. Check XDG_CURRENT_DESKTOP
	if xdg := os.Getenv("XDG_CURRENT_DESKTOP"); xdg != "" {
		normalized := normalizeDesktop(xdg)
		if normalized != "" {
			return normalized
		}
	}

	// 2. Check DESKTOP_SESSION
	if ds := os.Getenv("DESKTOP_SESSION"); ds != "" {
		normalized := normalizeDesktop(ds)
		if normalized != "" {
			return normalized
		}
	}

	// 3. Fallback: scan running processes
	return detectWMByProcess()
}

// detectWMByProcess reads /proc and checks for known WM process names.
func detectWMByProcess() string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return ""
	}

	// Limit to first 500 PIDs to avoid performance issues on servers
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name[0] < '0' || name[0] > '9' {
			continue // not a PID directory
		}

		count++
		if count > 500 {
			break
		}

		// Read /proc/<pid>/comm for the process name
		comm, err := os.ReadFile("/proc/" + name + "/comm")
		if err != nil {
			continue
		}
		procName := strings.TrimSpace(string(comm))
		if wm, ok := knownWMs[strings.ToLower(procName)]; ok {
			return wm
		}
	}
	return ""
}

// normalizeDesktop normalizes desktop environment / WM names.
func normalizeDesktop(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.Split(name, ":")[0] // handle "gnome:gnome-classic" etc.
	name = strings.Split(name, " ")[0]

	// Map common names to their canonical forms
	switch {
	case strings.Contains(name, "hyprland"):
		return "hyprland"
	case strings.Contains(name, "gnome"):
		return "gnome"
	case strings.Contains(name, "kde") || name == "kwin" || name == "plasma":
		return "kde"
	case strings.Contains(name, "xfce"):
		return "xfce"
	case strings.Contains(name, "cinnamon"):
		return "cinnamon"
	case strings.Contains(name, "mate"):
		return "mate"
	case strings.Contains(name, "sway"):
		return "sway"
	case strings.Contains(name, "i3"):
		return "i3"
	case strings.Contains(name, "bspwm"):
		return "bspwm"
	case strings.Contains(name, "qtile"):
		return "qtile"
	case strings.Contains(name, "river"):
		return "river"
	case strings.Contains(name, "awesome"):
		return "awesome"
	case strings.Contains(name, "dwm"):
		return "dwm"
	case strings.Contains(name, "budgie"):
		return "budgie"
	case strings.Contains(name, "deepin"):
		return "deepin"
	case strings.Contains(name, "cosmic"):
		return "cosmic"
	case strings.Contains(name, "niri"):
		return "niri"
	case strings.Contains(name, "leftwm"):
		return "leftwm"
	case strings.Contains(name, "openbox"):
		return "openbox"
	case strings.Contains(name, "fluxbox"):
		return "fluxbox"
	case strings.Contains(name, "lxqt"):
		return "lxqt"
	case strings.Contains(name, "lumina"):
		return "lumina"
	}

	// If nothing matches, return the raw name for extensibility
	return name
}
