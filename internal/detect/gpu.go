package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// sysRoot is the sysfs mount point; tests point it at a fake tree.
var sysRoot = "/sys"

// gpuVendors maps PCI vendor IDs of display controllers to layer names.
var gpuVendors = map[string]string{
	"0x1002": "amd",
	"0x8086": "intel",
	"0x10de": "nvidia",
	"0x1414": "microsoft", // Hyper-V synthetic video
	"0x15ad": "vmware",
	"0x1234": "qemu",
	"0x1af4": "virtio",
	"0x80ee": "oracle", // VirtualBox
}

// gpuPriority decides which GPU names the layer when several are present
// (hybrid laptops): a discrete card wins over an integrated one.
var gpuPriority = map[string]int{
	"nvidia": 3,
	"amd":    2,
	"intel":  1,
}

// detectGPU identifies the GPU vendor from DRM devices, falling back to the
// PCI bus (display controllers only, class 0x03xxxx).
func detectGPU() string {
	if gpu := pickGPU(drmVendors()); gpu != "" {
		return gpu
	}
	return pickGPU(pciDisplayVendors())
}

func pickGPU(vendors []string) string {
	best, bestPrio := "", -1
	for _, v := range vendors {
		name, ok := gpuVendors[strings.ToLower(strings.TrimSpace(v))]
		if !ok {
			continue
		}
		if p := gpuPriority[name]; p > bestPrio {
			best, bestPrio = name, p
		}
	}
	return best
}

func drmVendors() []string {
	base := filepath.Join(sysRoot, "class", "drm")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		// card0, card1... but not connectors like card0-HDMI-A-1
		if !strings.HasPrefix(name, "card") || strings.Contains(name, "-") {
			continue
		}
		if v, err := os.ReadFile(filepath.Join(base, name, "device", "vendor")); err == nil {
			out = append(out, string(v))
		}
	}
	return out
}

func pciDisplayVendors() []string {
	base := filepath.Join(sysRoot, "bus", "pci", "devices")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		class, err := os.ReadFile(filepath.Join(base, e.Name(), "class"))
		if err != nil || !strings.HasPrefix(strings.TrimSpace(string(class)), "0x03") {
			continue
		}
		if v, err := os.ReadFile(filepath.Join(base, e.Name(), "vendor")); err == nil {
			out = append(out, string(v))
		}
	}
	return out
}
