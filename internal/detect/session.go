package detect

import "os"

// detectSession checks environment variables to determine the display server.
// Returns "wayland", "x11", or empty string if unknown.
func detectSession() string {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return "wayland"
	}
	if os.Getenv("DISPLAY") != "" {
		return "x11"
	}
	return ""
}
