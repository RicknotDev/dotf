package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// gpuVendors maps PCI vendor IDs to canonical names.
var gpuVendors = map[string]string{
	"0x1002": "amd",
	"0x1022": "amd", // AMD
	"0x8086": "intel",
	"0x10de": "nvidia",
	"0x1ae0": "nvidia",    // NVIDIA
	"0x1414": "microsoft", // Hyper-V
	"0x15ad": "vmware",
	"0x1234": "qemu",
	"0x1af4": "virtio",
	"0x80ee": "oracle",
}

// detectGPU identifies the GPU vendor.
// Priority: /sys/class/drm > lspci fallback.
func detectGPU() string {
	// Read DRM device vendor IDs
	gpu := detectGPUFromDRM()
	if gpu != "" {
		return gpu
	}

	// Fallback: try reading from /proc/bus/pci/devices
	gpu = detectGPUFromPCI()
	if gpu != "" {
		return gpu
	}

	return ""
}

// detectGPUFromDRM reads vendor files from /sys/class/drm/.
func detectGPUFromDRM() string {
	entries, err := os.ReadDir("/sys/class/drm")
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "card") {
			continue
		}
		devicePath := filepath.Join("/sys/class/drm", name, "device/vendor")
		vendor, err := os.ReadFile(devicePath)
		if err != nil {
			continue
		}
		vendorStr := strings.TrimSpace(string(vendor))
		if gpu, ok := gpuVendors[strings.ToLower(vendorStr)]; ok {
			return gpu
		}
	}

	return ""
}

// detectGPUFromPCI reads GPU info from /proc/bus/pci/devices.
func detectGPUFromPCI() string {
	// Simplified: look for VGA-compatible controllers in the PCI device list
	data, err := os.ReadFile("/proc/bus/pci/devices")
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Check if the device name mentions known GPU brands
		lower := strings.ToLower(line)
		for _, gpu := range []string{"amd", "radeon", "advanced micro", "nvidia", "geforce", "intel", "arc"} {
			if strings.Contains(lower, gpu) {
				switch {
				case strings.Contains(lower, "amd") || strings.Contains(lower, "radeon") || strings.Contains(lower, "advanced micro"):
					return "amd"
				case strings.Contains(lower, "nvidia") || strings.Contains(lower, "geforce"):
					return "nvidia"
				case strings.Contains(lower, "intel") || strings.Contains(lower, "arc"):
					return "intel"
				}
			}
		}
	}

	return ""
}
