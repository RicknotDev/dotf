package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeDesktop(t *testing.T) {
	cases := map[string]string{
		"ubuntu:GNOME":  "gnome",
		"GNOME":         "gnome",
		"X-Cinnamon":    "cinnamon",
		"KDE":           "kde",
		"Hyprland":      "hyprland",
		"sway":          "sway",
		"pop:GNOME":     "gnome",
		"something-new": "something-new",
	}
	for in, want := range cases {
		if got := normalizeDesktop(in); got != want {
			t.Errorf("normalizeDesktop(%q) = %q, want %q", in, got, want)
		}
	}
}

func fakeSys(t *testing.T, files map[string]string) {
	t.Helper()
	root := t.TempDir()
	for p, c := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := sysRoot
	sysRoot = root
	t.Cleanup(func() { sysRoot = old })
}

func TestDetectGPUHybridPrefersDiscrete(t *testing.T) {
	fakeSys(t, map[string]string{
		"class/drm/card0/device/vendor": "0x8086",
		"class/drm/card1/device/vendor": "0x10de",
		"class/drm/card0-eDP-1/status":  "connected",
	})
	if got := detectGPU(); got != "nvidia" {
		t.Fatalf("hybrid intel+nvidia: got %q, want nvidia", got)
	}
}

func TestDetectGPUIgnoresNonGPUVendors(t *testing.T) {
	// 0x1ae0 is Google and 0x1022 is AMD's chipset vendor ID; neither is a GPU.
	fakeSys(t, map[string]string{
		"class/drm/card0/device/vendor": "0x1ae0",
	})
	if got := detectGPU(); got != "" {
		t.Fatalf("got %q, want no GPU", got)
	}
}

func TestDetectGPUPCIFallbackUsesClass(t *testing.T) {
	fakeSys(t, map[string]string{
		// AMD chipset (class 0x0600, host bridge) must not count as a GPU
		"bus/pci/devices/0000:00:00.0/class":  "0x060000",
		"bus/pci/devices/0000:00:00.0/vendor": "0x1022",
		// Intel VGA controller
		"bus/pci/devices/0000:00:02.0/class":  "0x030000",
		"bus/pci/devices/0000:00:02.0/vendor": "0x8086",
	})
	if got := detectGPU(); got != "intel" {
		t.Fatalf("got %q, want intel", got)
	}
}

func TestIsVMFromDMI(t *testing.T) {
	cases := []struct {
		product, vendor string
		vm              bool
	}{
		{"surface laptop 5", "microsoft corporation", false},
		{"virtual machine", "microsoft corporation", true},
		{"standard pc (q35 + ich9, 2009)", "qemu", true},
		{"virtualbox", "innotek gmbh", true},
		{"thinkpad x1 carbon", "lenovo", false},
	}
	for _, c := range cases {
		if got := isVMFromDMI(c.product, c.vendor); got != c.vm {
			t.Errorf("isVMFromDMI(%q, %q) = %v, want %v", c.product, c.vendor, got, c.vm)
		}
	}
}

func TestTerminalIgnoresMultiplexers(t *testing.T) {
	t.Setenv("TERMINAL", "")
	t.Setenv("TERM", "xterm-256color")
	for _, tp := range []string{"tmux", "vscode"} {
		t.Setenv("TERM_PROGRAM", tp)
		if got := detectTerminal(); got == tp {
			t.Fatalf("TERM_PROGRAM=%s taken as terminal", tp)
		}
	}
	t.Setenv("TERM_PROGRAM", "WezTerm")
	if got := detectTerminal(); got != "wezterm" {
		t.Fatalf("got %q, want wezterm", got)
	}
}

func TestShellFallbackUsesParentProcess(t *testing.T) {
	t.Setenv("SHELL", "")
	// The parent of the test binary is `go test` or a shell; either way the
	// fallback must read a real process name instead of returning "".
	if _, err := os.Stat("/proc/self/comm"); err != nil {
		t.Skip("no /proc")
	}
	if got := detectShell(); got == "" {
		t.Fatal("fallback returned empty shell")
	}
}
