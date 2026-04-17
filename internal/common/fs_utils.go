package common

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
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
