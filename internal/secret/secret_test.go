package secret

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSecretsFindsAgeFiles(t *testing.T) {
	dir := t.TempDir()

	// Create layers/<layer>/secrets/ structure (matching the real directory layout)
	layerDir := filepath.Join(dir, "layers", "base", "secrets")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an age-encrypted secret file
	secretFile := filepath.Join(layerDir, "github_token.age")
	if err := os.WriteFile(secretFile, []byte("age-encrypted-content"), 0644); err != nil {
		t.Fatal(err)
	}

	// DiscoverSecrets now takes layersDir (the layers/ directory) and layer paths
	layersDir := filepath.Join(dir, "layers")
	secrets := DiscoverSecrets(layersDir, []string{"base"})

	if len(secrets) == 0 {
		t.Fatal("expected to find secrets in base layer")
	}

	found := false
	for _, s := range secrets {
		if s.Name == "github_token" && s.Method == "age" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to find github_token.age secret, got: %+v", secrets)
	}
}

func TestDiscoverSecretsFindsGPGFiles(t *testing.T) {
	dir := t.TempDir()

	layerDir := filepath.Join(dir, "layers", "base", "secrets")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}

	secretFile := filepath.Join(layerDir, "ssh_key.gpg")
	if err := os.WriteFile(secretFile, []byte("gpg-encrypted-content"), 0644); err != nil {
		t.Fatal(err)
	}

	layersDir := filepath.Join(dir, "layers")
	secrets := DiscoverSecrets(layersDir, []string{"base"})

	if len(secrets) == 0 {
		t.Fatal("expected to find secrets in base layer")
	}

	found := false
	for _, s := range secrets {
		if s.Name == "ssh_key" && s.Method == "gpg" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected to find ssh_key.gpg secret, got: %+v", secrets)
	}
}

func TestDiscoverSecretsIgnoresNonSecretFiles(t *testing.T) {
	dir := t.TempDir()

	layerDir := filepath.Join(dir, "layers", "base", "secrets")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a plain text file (should be ignored)
	if err := os.WriteFile(filepath.Join(layerDir, "notes.txt"), []byte("plain text"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a .yaml file (should be ignored)
	if err := os.WriteFile(filepath.Join(layerDir, "config.yaml"), []byte("key: value"), 0644); err != nil {
		t.Fatal(err)
	}

	layersDir := filepath.Join(dir, "layers")
	secrets := DiscoverSecrets(layersDir, []string{"base"})

	if len(secrets) != 0 {
		t.Fatalf("expected no secrets for non-secret files, got %d: %+v", len(secrets), secrets)
	}
}

// TestDiscoverSecretsWithWrongPath verifies that passing the repo root
// (without layers/) does NOT find secrets — this was the original bug.
func TestDiscoverSecretsWithWrongPath(t *testing.T) {
	dir := t.TempDir()

	// Create layers/<layer>/secrets/ structure
	layerDir := filepath.Join(dir, "layers", "base", "secrets")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}

	secretFile := filepath.Join(layerDir, "token.age")
	if err := os.WriteFile(secretFile, []byte("encrypted"), 0644); err != nil {
		t.Fatal(err)
	}

	// If we pass repoRoot instead of layersDir, secrets should NOT be found
	// (this was the old buggy behavior)
	oldBuggySecrets := DiscoverSecrets(dir, []string{"base"})
	if len(oldBuggySecrets) != 0 {
		t.Fatalf("secrets should not be found when passing repo root without layers/: got %d", len(oldBuggySecrets))
	}

	// With correct layersDir, secrets SHOULD be found
	layersDir := filepath.Join(dir, "layers")
	fixedSecrets := DiscoverSecrets(layersDir, []string{"base"})
	if len(fixedSecrets) == 0 {
		t.Fatal("secrets should be found when passing layersDir correctly")
	}
}

func TestDiscoverSecretsEmptyLayerPath(t *testing.T) {
	dir := t.TempDir()

	// No secrets directory at all
	layersDir := filepath.Join(dir, "layers")
	os.MkdirAll(layersDir, 0755)

	secrets := DiscoverSecrets(layersDir, []string{"base"})
	if len(secrets) != 0 {
		t.Fatalf("expected no secrets for layer without secrets/ dir, got %d", len(secrets))
	}
}

func TestDiscoverSecretsMultipleLayers(t *testing.T) {
	dir := t.TempDir()

	// Create secrets in multiple layers
	for _, layer := range []string{"base", "shell/zsh"} {
		layerDir := filepath.Join(dir, "layers", layer, "secrets")
		if err := os.MkdirAll(layerDir, 0755); err != nil {
			t.Fatal(err)
		}
		secretFile := filepath.Join(layerDir, "token.age")
		if err := os.WriteFile(secretFile, []byte("encrypted-"+layer), 0644); err != nil {
			t.Fatal(err)
		}
	}

	layersDir := filepath.Join(dir, "layers")
	secrets := DiscoverSecrets(layersDir, []string{"base", "shell/zsh"})

	if len(secrets) != 2 {
		t.Fatalf("expected 2 secrets across layers, got %d: %+v", len(secrets), secrets)
	}
}

func TestDiscoverSecretsNamesWithDots(t *testing.T) {
	dir := t.TempDir()

	layerDir := filepath.Join(dir, "layers", "base", "secrets")
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Secret name with dots should strip only the extension
	if err := os.WriteFile(filepath.Join(layerDir, "my.secret.key.age"), []byte("encrypted"), 0644); err != nil {
		t.Fatal(err)
	}

	layersDir := filepath.Join(dir, "layers")
	secrets := DiscoverSecrets(layersDir, []string{"base"})

	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}
	if secrets[0].Name != "my.secret.key" {
		t.Fatalf("expected name 'my.secret.key', got '%s'", secrets[0].Name)
	}
}
