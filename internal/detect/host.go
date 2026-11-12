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
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(sysRoot, "class", "dmi", "id", name))
		if err != nil {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(string(data)))
	}
	if isVMFromDMI(read("product_name"), read("sys_vendor")) {
		return true
	}

	// The CPU reports a hypervisor
	cpuinfo, err := os.ReadFile("/proc/cpuinfo")
	return err == nil && strings.Contains(string(cpuinfo), " hypervisor")
}

// isVMFromDMI decides from DMI strings (lowercased). Microsoft also makes
// physical machines (Surface), so its vendor string alone is not enough:
// Hyper-V guests report the product "Virtual Machine".
func isVMFromDMI(product, vendor string) bool {
	for _, s := range []string{product, vendor} {
		for _, vm := range []string{"virtualbox", "vmware", "qemu", "kvm", "bochs", "xen", "parallels", "bhyve"} {
			if strings.Contains(s, vm) {
				return true
			}
		}
	}
	return strings.Contains(vendor, "microsoft") && strings.Contains(product, "virtual machine")
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
