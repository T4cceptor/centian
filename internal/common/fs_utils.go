package common

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	secureDirPerm  = 0o750
	secureFilePerm = 0o600
)

// ReadJSONFile reads JSON from disk into target.
func ReadJSONFile(path string, target any) error {
	// #nosec G304 -- caller controls the path; this helper intentionally reads repository/runtime files by path.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// ReadYAMLFile reads YAML from disk into target.
func ReadYAMLFile(path string, target any) error {
	// #nosec G304 -- caller controls the path; this helper intentionally reads repository/runtime files by path.
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read %q: %w", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to parse %q: %w", path, err)
	}
	return nil
}

// WriteJSONFile writes pretty-printed JSON and creates parent directories as needed.
func WriteJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), secureDirPerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, secureFilePerm)
}

// CopyDir recursively copies a directory tree into dst.
func CopyDir(src, dst string) error {
	root, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()

	return copyRootTree(root.FS(), ".", dst)
}

func copyRootTree(srcFS fs.FS, walkPath, dst string) error {
	targetDir := copyTargetPath(dst, walkPath)
	if err := os.MkdirAll(targetDir, secureDirPerm); err != nil {
		return err
	}

	entries, err := fs.ReadDir(srcFS, walkPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		childPath := entry.Name()
		if walkPath != "." {
			childPath = filepath.Join(walkPath, entry.Name())
		}
		if entry.IsDir() {
			if err := copyRootTree(srcFS, childPath, dst); err != nil {
				return err
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := fs.ReadFile(srcFS, childPath)
		if err != nil {
			return err
		}
		target := copyTargetPath(dst, childPath)
		if err := os.MkdirAll(filepath.Dir(target), secureDirPerm); err != nil {
			return err
		}
		if err := writeCopiedFile(target, data, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

// CopyFile copies one file into the destination path, creating parent directories first.
func CopyFile(src, dst string) error {
	// #nosec G304 -- caller controls the path; this helper intentionally copies configured files by path.
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), secureDirPerm); err != nil {
		return err
	}
	return writeCopiedFile(dst, data, 0o644)
}

func copyTargetPath(dst, walkPath string) string {
	if walkPath == "." {
		return dst
	}
	return filepath.Join(dst, walkPath)
}

func writeCopiedFile(path string, data []byte, mode os.FileMode) error {
	perms := mode.Perm()
	if perms == 0 {
		perms = secureFilePerm
	}
	// #nosec G306,G703 -- target path is derived from validated copy inputs; copied fixtures may intentionally preserve file mode bits.
	return os.WriteFile(path, data, perms)
}

// ResolvePathUnderRoot resolves a relative path and rejects paths escaping the named root.
func ResolvePathUnderRoot(root, relativePath, fieldName, rootName string) (string, error) {
	scope := strings.TrimSpace(rootName)
	if scope == "" {
		scope = "root"
	}
	trimmed := strings.TrimSpace(relativePath)
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	cleaned := filepath.Clean(trimmed)
	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("%s %q must be relative", fieldName, relativePath)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within the %s", fieldName, relativePath, scope)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s %q: %w", scope, root, err)
	}
	resolved := filepath.Join(rootAbs, cleaned)
	rel, err := filepath.Rel(rootAbs, resolved)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s %q: %w", fieldName, relativePath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s %q must stay within the %s", fieldName, relativePath, scope)
	}
	return resolved, nil
}

// ResolveExistingFileUnderRoot validates that relativePath exists under root and is a file.
func ResolveExistingFileUnderRoot(root, relativePath, fieldName, rootName string) (string, error) {
	resolved, err := ResolvePathUnderRoot(root, relativePath, fieldName, rootName)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%s %q does not exist: %w", fieldName, relativePath, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s %q must be a file", fieldName, relativePath)
	}
	return resolved, nil
}

// ResolveExistingDirUnderRoot validates that relativePath exists under root and is a directory.
func ResolveExistingDirUnderRoot(root, relativePath, fieldName, rootName string) (string, error) {
	resolved, err := ResolvePathUnderRoot(root, relativePath, fieldName, rootName)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%s %q does not exist: %w", fieldName, relativePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s %q must be a directory", fieldName, relativePath)
	}
	return resolved, nil
}

// EnsureExistingPathUnderRoot validates that relativePath exists under root.
func EnsureExistingPathUnderRoot(root, relativePath, fieldName, rootName string) error {
	resolved, err := ResolvePathUnderRoot(root, relativePath, fieldName, rootName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(resolved); err != nil {
		return fmt.Errorf("%s %q does not exist: %w", fieldName, relativePath, err)
	}
	return nil
}
