package cmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func copyEmbeddedBinary(name string, data []byte, version string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}
	binaryPath := filepath.Join(cwd, name)

	if _, err := os.Stat(binaryPath); err == nil {
		fmt.Printf("%s already exists at %s\n", name, binaryPath)
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to check existing %s binary: %w", name, err)
	}

	if len(data) == 0 {
		return fmt.Errorf("embedded %s binary is empty", name)
	}

	if err := writeFileAtomically(binaryPath, data, 0o755); err != nil {
		return fmt.Errorf("failed to write %s binary: %w", name, err)
	}

	fmt.Printf("%s %s copied to %s\n", name, version, binaryPath)
	return nil
}

func writeFileAtomically(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func copyEmbeddedTree(fsys fs.FS, root, dest string) error {
	return fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}

		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		src, err := fsys.Open(path)
		if err != nil {
			return fmt.Errorf("open embedded %q: %w", path, err)
		}
		defer src.Close()

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create directory for %q: %w", target, err)
		}

		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return nil
			}
			return fmt.Errorf("create %q: %w", target, err)
		}

		if _, err := io.Copy(out, src); err != nil {
			_ = out.Close()
			_ = os.Remove(target)
			return fmt.Errorf("write %q: %w", target, err)
		}
		if err := out.Close(); err != nil {
			_ = os.Remove(target)
			return fmt.Errorf("close %q: %w", target, err)
		}
		return nil
	})
}
