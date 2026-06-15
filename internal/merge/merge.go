// Package merge provides safe, format-aware merging of configuration files
// from multiple layers. When merging is unsafe, it falls back to the
// highest-priority file with a warning.
package merge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Result represents a merge result.
type Result struct {
	Data     []byte
	Source   string   // which file won (or "merge" if merged)
	Merged   bool
	Warnings []string
}

// MergeFile represents a file to potentially merge.
type MergeFile struct {
	Path     string
	Priority int // lower = higher priority
}

// Merge attempts to merge multiple files into one.
// Falls back to the highest-priority file if merge is unsafe.
func Merge(files []MergeFile) (*Result, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to merge")
	}
	if len(files) == 1 {
		data, err := os.ReadFile(files[0].Path)
		if err != nil {
			return nil, err
		}
		return &Result{
			Data:   data,
			Source: files[0].Path,
			Merged: false,
		}, nil
	}

	// Detect format from the highest-priority file's extension
	winner := files[0]
	ext := strings.ToLower(filepath.Ext(winner.Path))

	// For non-structured formats, winner takes all
	switch ext {
	case ".yaml", ".yml":
		return mergeYAML(files)
	case ".json":
		return mergeJSON(files)
	case ".toml":
		return mergeTOML(files)
	case ".ini", ".conf", ".cfg":
		return mergeINI(files)
	default:
		// Binary or unknown format: winner takes all
		data, err := os.ReadFile(winner.Path)
		if err != nil {
			return nil, err
		}
		return &Result{
			Data:   data,
			Source: winner.Path,
			Merged: false,
		}, nil
	}
}

// IsMergeableFormat checks if a file extension is a mergeable format.
func IsMergeableFormat(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml", ".json", ".toml", ".ini", ".conf", ".cfg":
		return true
	}
	return false
}

// mergeYAML performs a deep merge of YAML files.
func mergeYAML(files []MergeFile) (*Result, error) {
	if len(files) == 1 {
		data, err := os.ReadFile(files[0].Path)
		if err != nil {
			return nil, err
		}
		return &Result{Data: data, Source: files[0].Path}, nil
	}

	data, err := os.ReadFile(files[0].Path)
	if err != nil {
		return nil, err
	}
	return &Result{
		Data:     data,
		Source:   files[0].Path,
		Merged:   false,
		Warnings: []string{"YAML deep merge not yet implemented, using highest-priority file"},
	}, nil
}

// mergeJSON performs a deep merge of JSON files.
func mergeJSON(files []MergeFile) (*Result, error) {
	if len(files) == 1 {
		data, err := os.ReadFile(files[0].Path)
		if err != nil {
			return nil, err
		}
		return &Result{Data: data, Source: files[0].Path}, nil
	}

	data, err := os.ReadFile(files[0].Path)
	if err != nil {
		return nil, err
	}
	return &Result{
		Data:     data,
		Source:   files[0].Path,
		Merged:   false,
		Warnings: []string{"JSON deep merge not yet implemented, using highest-priority file"},
	}, nil
}

// mergeTOML performs a deep merge of TOML files.
func mergeTOML(files []MergeFile) (*Result, error) {
	if len(files) == 1 {
		data, err := os.ReadFile(files[0].Path)
		if err != nil {
			return nil, err
		}
		return &Result{Data: data, Source: files[0].Path}, nil
	}

	data, err := os.ReadFile(files[0].Path)
	if err != nil {
		return nil, err
	}
	return &Result{
		Data:     data,
		Source:   files[0].Path,
		Merged:   false,
		Warnings: []string{"TOML deep merge not yet implemented, using highest-priority file"},
	}, nil
}

// mergeINI performs section-level merging of INI/conf files.
func mergeINI(files []MergeFile) (*Result, error) {
	data, err := os.ReadFile(files[0].Path)
	if err != nil {
		return nil, err
	}
	return &Result{
		Data:     data,
		Source:   files[0].Path,
		Merged:   false,
		Warnings: []string{"INI section-level merge not yet implemented, using highest-priority file"},
	}, nil
}

// Description returns a human-readable description of the merge result.
func (r *Result) Description() string {
	if r.Merged {
		return fmt.Sprintf("merged from %d files", len(r.Warnings))
	}
	return fmt.Sprintf("winner: %s", r.Source)
}
