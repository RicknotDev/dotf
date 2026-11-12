// Package secret deploys encrypted files (age or GPG) from the repository.
//
// layers/<layer>/secrets/<path>.age (or .gpg / .asc) is decrypted to
// ~/<path> with mode 0600. Decryption writes straight into a temporary file
// created with mode 0600 in the destination directory, which is then
// renamed into place; the plaintext never goes to /tmp.
package secret

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Secret describes an encrypted secret file.
type Secret struct {
	Name      string // destination path relative to $HOME (e.g. ".config/gh/hosts.yml")
	Encrypted string // absolute path of the encrypted file
	Method    string // "age" or "gpg"
	Layer     string // layer path ("base", "host/laptop", ...)
}

// Methods by file extension.
var extensions = map[string]string{
	".age": "age",
	".gpg": "gpg",
	".asc": "gpg",
}

// Discover returns the secrets in a single layer directory.
func Discover(layerDir, layerPath string) []Secret {
	root := filepath.Join(layerDir, "secrets")
	var secrets []Secret
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		method, ok := extensions[strings.ToLower(filepath.Ext(path))]
		if !ok {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		secrets = append(secrets, Secret{
			Name:      filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel))),
			Encrypted: path,
			Method:    method,
			Layer:     layerPath,
		})
		return nil
	})
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })
	return secrets
}

// DiscoverSecrets returns the secrets of the given layers (layer paths
// relative to layersDir, lowest priority first).
func DiscoverSecrets(layersDir string, layerPaths []string) []Secret {
	var all []Secret
	for _, l := range layerPaths {
		all = append(all, Discover(filepath.Join(layersDir, l), l)...)
	}
	return all
}

// Decrypter decrypts a secret into w. Replaced in tests.
var Decrypter = decrypt

// Decrypt writes the plaintext of s to w.
func Decrypt(s Secret, w io.Writer) error {
	return Decrypter(s, w)
}

func decrypt(s Secret, w io.Writer) error {
	var cmd *exec.Cmd
	switch s.Method {
	case "age":
		keys, err := ageIdentities()
		if err != nil {
			return err
		}
		args := []string{"--decrypt"}
		for _, k := range keys {
			args = append(args, "-i", k)
		}
		cmd = exec.Command("age", append(args, s.Encrypted)...)
	case "gpg":
		cmd = exec.Command("gpg", "--batch", "--quiet", "--decrypt", s.Encrypted)
	default:
		return fmt.Errorf("unknown encryption method: %s", s.Method)
	}
	var stderr bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%s decryption failed: %w: %s", s.Method, err, msg)
		}
		return fmt.Errorf("%s decryption failed: %w", s.Method, err)
	}
	return nil
}

// ageIdentities returns the age identity files to try:
// ~/.config/dotf/keys/*.age, then ~/.config/age/*.txt.
func ageIdentities() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	keyDir := filepath.Join(homeDir, ".config", "dotf", "keys")
	keys, _ := filepath.Glob(filepath.Join(keyDir, "*.age"))
	alt, _ := filepath.Glob(filepath.Join(homeDir, ".config", "age", "*.txt"))
	keys = append(keys, alt...)
	if len(keys) == 0 {
		return nil, fmt.Errorf("no age identity found (looked in %s and ~/.config/age/*.txt)", keyDir)
	}
	return keys, nil
}
