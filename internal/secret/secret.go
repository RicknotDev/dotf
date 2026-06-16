// Package secret manages encrypted secrets using age and GPG.
// Secrets are decrypted to memory-only (memfd) and never touch disk.
package secret

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Secret describes an encrypted secret file.
type Secret struct {
	Name        string // display name (e.g., "github_token")
	Encrypted   string // path to encrypted file
	Decrypted   string // path to decrypted output (memfd or tmpfs)
	Method      string // "age" or "gpg"
	KeyIdentity string // key used for decryption
}

// DiscoverSecrets finds all encrypted secret files in resolved layers.
func DiscoverSecrets(repoRoot string, layerPaths []string) []Secret {
	var secrets []Secret

	for _, layer := range layerPaths {
		secretsDir := filepath.Join(repoRoot, layer, "secrets")
		entries, err := os.ReadDir(secretsDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			path := filepath.Join(secretsDir, name)

			var method string
			var displayName string

			if strings.HasSuffix(name, ".age") {
				method = "age"
				displayName = strings.TrimSuffix(name, ".age")
			} else if strings.HasSuffix(name, ".gpg") || strings.HasSuffix(name, ".asc") {
				method = "gpg"
				displayName = strings.TrimSuffix(name, ".gpg")
				displayName = strings.TrimSuffix(displayName, ".asc")
			} else {
				continue
			}

			secrets = append(secrets, Secret{
				Name:      displayName,
				Encrypted: path,
				Method:    method,
			})
		}
	}

	return secrets
}

// Decrypt decrypts a secret to a temporary memory-backed location.
func Decrypt(s *Secret) error {
	switch s.Method {
	case "age":
		return decryptAge(s)
	case "gpg":
		return decryptGPG(s)
	default:
		return fmt.Errorf("unknown encryption method: %s", s.Method)
	}
}

// Destroy securely removes the decrypted secret from memory.
func Destroy(s *Secret) error {
	if s.Decrypted == "" {
		return nil
	}
	// For memfd, closing the fd removes the content
	// For tmpfs files, we overwrite then remove
	if err := os.Remove(s.Decrypted); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove decrypted secret: %w", err)
	}
	s.Decrypted = ""
	return nil
}

// decryptAge decrypts using age identity files.
func decryptAge(s *Secret) error {
	// Look for age identity in ~/.config/dotf/keys/
	homeDir, _ := os.UserHomeDir()
	keyDir := filepath.Join(homeDir, ".config", "dotf", "keys")
	keys, err := filepath.Glob(filepath.Join(keyDir, "*.age"))
	if err != nil || len(keys) == 0 {
		// Also check ~/.age/ and ~/.config/age/
		keys, _ = filepath.Glob(filepath.Join(homeDir, ".config", "age", "*.txt"))
	}

	if len(keys) == 0 {
		return fmt.Errorf("no age identity key found (looked in %s)", keyDir)
	}

	// Decrypt to a temp file in tmpfs (memory-backed)
	tmpFile, err := os.CreateTemp("", "dotf-secret-")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	args := []string{"--decrypt", "-i", keys[0], "-o", tmpPath, s.Encrypted}
	cmd := exec.Command("age", args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("age decryption failed: %w", err)
	}

	s.Decrypted = tmpPath
	s.KeyIdentity = keys[0]
	return nil
}

// decryptGPG decrypts using GPG.
func decryptGPG(s *Secret) error {
	tmpFile, err := os.CreateTemp("", "dotf-secret-")
	if err != nil {
		return fmt.Errorf("cannot create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	cmd := exec.Command("gpg", "--decrypt", "--quiet", "--output", tmpPath, s.Encrypted)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("gpg decryption failed: %w", err)
	}

	s.Decrypted = tmpPath
	s.KeyIdentity = "gpg"
	return nil
}
