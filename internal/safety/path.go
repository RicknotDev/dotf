// Package safety provides path validation and symlink security for DOTF.
// All filesystem paths are validated through this layer before any operation.
package safety

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SensitivePaths are system paths that must never be targeted. Entries that
// are the home directory itself or one of its parents are ignored (see
// ValidateTargetPath).
var SensitivePaths = []string{
	"/etc/passwd",
	"/etc/shadow",
	"/etc/sudoers",
	"/etc/sudoers.d",
	"/etc/securetty",
	"/root",
	"/boot",
	"/dev",
	"/proc",
	"/sys",
	"/bin",
	"/sbin",
	"/usr/bin",
	"/usr/sbin",
}

// MaxSymlinkChain is the maximum number of symlinks to follow.
const MaxSymlinkChain = 40

// MaxFilePath is the maximum reasonable file path length.
const MaxFilePath = 4096

// PathValidationResult contains the result of a path validation.
type PathValidationResult struct {
	Safe       bool
	Reason     string
	Normalized string // Cleaned absolute path
}

// ValidateLayerFile validates a file path within a repository layer.
func ValidateLayerFile(repoRoot, layerFile string) *PathValidationResult {
	// 1. Clean and absolutize paths
	cleanRepo := filepath.Clean(repoRoot)
	absRepo, err := filepath.Abs(cleanRepo)
	if err != nil {
		return fail("cannot resolve repository root: %v", err)
	}

	cleanFile := filepath.Clean(layerFile)
	absFile, err := filepath.Abs(cleanFile)
	if err != nil {
		return fail("cannot resolve layer file: %v", err)
	}

	// 2. Verify file is within the repository
	rel, err := filepath.Rel(absRepo, absFile)
	if err != nil {
		return fail("cannot compute relative path: %v", err)
	}
	if escapes(filepath.ToSlash(rel)) {
		return fail("path escapes repository root: %s", rel)
	}

	// 3. Check file exists and is a regular file (allow symlinks to regular files)
	info, err := os.Lstat(absFile)
	if err != nil {
		return fail("layer file does not exist: %s", absFile)
	}

	// 4. If it's a symlink, follow and validate the chain
	if info.Mode()&os.ModeSymlink != 0 {
		visited := make(map[string]bool)
		current := absFile
		for i := 0; i < MaxSymlinkChain; i++ {
			if visited[current] {
				return fail("symlink loop detected at %s", current)
			}
			visited[current] = true

			target, err := os.Readlink(current)
			if err != nil {
				return fail("cannot read symlink %s: %v", current, err)
			}

			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			target = filepath.Clean(target)

			// Must stay within repository
			targetRel, err := filepath.Rel(absRepo, target)
			if err != nil || escapes(filepath.ToSlash(targetRel)) {
				return fail("symlink escapes repository: %s -> %s", current, target)
			}

			targetInfo, err := os.Lstat(target)
			if err != nil {
				return fail("symlink target missing: %s -> %s", current, target)
			}

			if targetInfo.Mode()&os.ModeSymlink == 0 {
				break // reached a real file
			}
			current = target
		}
	}

	return &PathValidationResult{
		Safe:       true,
		Normalized: absFile,
	}
}

// homeRelativeBlocked lists paths (relative to $HOME) that DOTF never
// writes, because doing so grants access to the account.
var homeRelativeBlocked = []string{
	".ssh/authorized_keys",
	".ssh/authorized_keys2",
}

// escapes reports whether a cleaned relative path leaves its base directory.
// "..foo" is a valid file name and does not escape.
func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../")
}

// ValidateTargetPath validates where a file will be installed in $HOME.
func ValidateTargetPath(homeDir, relativePath string) *PathValidationResult {
	cleanHome := filepath.Clean(homeDir)
	absHome, err := filepath.Abs(cleanHome)
	if err != nil {
		return fail("cannot resolve home directory: %v", err)
	}

	if filepath.IsAbs(relativePath) {
		return fail("relative path is absolute: %s", relativePath)
	}
	cleanRel := filepath.ToSlash(filepath.Clean(relativePath))
	if escapes(cleanRel) || cleanRel == "." {
		return fail("relative path escapes: %s", relativePath)
	}

	target := filepath.Join(absHome, cleanRel)
	targetRel, err := filepath.Rel(absHome, target)
	if err != nil {
		return fail("cannot compute target path: %v", err)
	}
	if escapes(filepath.ToSlash(targetRel)) {
		return fail("target escapes home directory: %s", target)
	}

	if len(target) > MaxFilePath {
		return fail("path too long (%d chars, max %d)", len(target), MaxFilePath)
	}

	for _, blocked := range homeRelativeBlocked {
		if cleanRel == blocked {
			return fail("refusing to manage %s (grants login access)", blocked)
		}
	}

	// System paths only matter when they are not the home directory itself
	// or one of its parents: for root, $HOME is /root and every target is
	// necessarily inside it.
	for _, sp := range SensitivePaths {
		absSP, _ := filepath.Abs(sp)
		if absSP == absHome || strings.HasPrefix(absHome, absSP+"/") {
			continue
		}
		if target == absSP || strings.HasPrefix(target, absSP+"/") {
			return fail("target is a sensitive system path: %s", sp)
		}
	}

	return &PathValidationResult{
		Safe:       true,
		Normalized: target,
	}
}

// IsSensitivePath checks if a path is considered sensitive.
func IsSensitivePath(path string) bool {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return false
	}
	for _, sp := range SensitivePaths {
		absSP, _ := filepath.Abs(sp)
		if abs == absSP || strings.HasPrefix(abs, absSP+"/") {
			return true
		}
	}
	return false
}

func fail(format string, args ...interface{}) *PathValidationResult {
	return &PathValidationResult{
		Safe:   false,
		Reason: fmt.Sprintf(format, args...),
	}
}
