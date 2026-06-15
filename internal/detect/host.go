package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// detectHostname returns the system hostname, lowercased.
func detectHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	// Remove domain suffix if present
	if idx := strings.Index(hostname, "."); idx > 0 {
		hostname = hostname[:idx]
	}
	return hostname
}

// detectDeviceType identifies whether the system is a laptop, desktop, server, VM, or container.
func detectDeviceType() string {
	// 1. Check for container environments first
	if isContainer() {
		return "container"
	}

	// 2. Check for VM environments
	if isVM() {
		return "vm"
	}

	// 3. Check for laptop (has battery)
	if hasBattery() {
		return "laptop"
	}

	// 4. Check for server (no display, likely headless)
	if !hasDisplay() {
		return "server"
	}

	// 5. Default to desktop
	return "desktop"
}

// isContainer checks for indicators of containerization.
func isContainer() bool {
	// Docker
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Podman
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	return false
}

// isVM checks for indicators of virtualisation.
func isVM() bool {
	// Check DMI product name for common hypervisors
	paths := []string{
		"/sys/class/dmi/id/product_name",
		"/sys/class/dmi/id/sys_vendor",
		"/sys/devices/virtual/dmi/id/product_name",
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(string(data)))
		for _, vm := range []string{"virtualbox", "vmware", "qemu", "kvm", "bochs", "xen", "microsoft", "hyper-v", "parallels"} {
			if strings.Contains(name, vm) {
				return true
			}
		}
	}

	// Also check /proc/cpuinfo for hypervisor flag
	cpuinfo, err := os.ReadFile("/proc/cpuinfo")
	if err == nil && strings.Contains(strings.ToLower(string(cpuinfo)), "hypervisor") {
		return true
	}

	return false
}

// hasBattery checks if the system has any power supply battery.
func hasBattery() bool {
	entries, err := os.ReadDir("/sys/class/power_supply")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		ueventPath := filepath.Join("/sys/class/power_supply", entry.Name(), "uevent")
		data, err := os.ReadFile(ueventPath)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "POWER_SUPPLY_TYPE=Battery") {
			return true
		}
	}
	return false
}

// hasDisplay checks if the system has a display server running.
func hasDisplay() bool {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return false
}
