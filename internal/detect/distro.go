package detect

import (
	"os"
	"strings"
)

// detectDistro reads /etc/os-release and returns the distro ID.
// Returns empty string if detection fails (unknown environment).
func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		// Fallback: try /usr/lib/os-release
		data, err = os.ReadFile("/usr/lib/os-release")
		if err != nil {
			return ""
		}
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			val := strings.TrimPrefix(line, "ID=")
			val = strings.Trim(val, `"'`)
			return strings.TrimSpace(val)
		}
	}
	return ""
}
