package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"llm/internal/tools"
)

// File describes one path written (or skipped) during an install.
type File struct {
	Name    string
	Path    string
	Version string
	Copied  bool
}

// Result lists the files considered while installing a binary.
type Result struct {
	Files []File
}

// ToDir copies bin and any sidecars into dir. Existing files are left in place.
func ToDir(dir string, bin tools.Binary) (Result, error) {
	var res Result
	if bin.Name == "" {
		return res, fmt.Errorf("binary name is empty")
	}
	if len(bin.Data) == 0 {
		return res, fmt.Errorf("embedded %s binary is empty", bin.Name)
	}

	file, err := writeFile(dir, bin.Name, bin.Data, 0o755)
	if err != nil {
		return res, err
	}
	file.Version = bin.Version
	res.Files = append(res.Files, file)

	for _, extra := range bin.ExtraFiles {
		if extra.Name == "" {
			return res, fmt.Errorf("%s sidecar name is empty", bin.Name)
		}
		if len(extra.Data) == 0 {
			return res, fmt.Errorf("embedded %s sidecar is empty", extra.Name)
		}
		file, err := writeFile(dir, extra.Name, extra.Data, 0o755)
		if err != nil {
			return res, err
		}
		file.Version = bin.Version
		res.Files = append(res.Files, file)
	}

	for _, extra := range bin.ExtraDirs {
		if extra.Name == "" {
			return res, fmt.Errorf("%s data directory name is empty", bin.Name)
		}
		dest := filepath.Join(dir, extra.Name)
		copied, err := copyTree(extra.FS, extra.Root, dest)
		if err != nil {
			return res, fmt.Errorf("copy %s: %w", extra.Name, err)
		}
		res.Files = append(res.Files, File{
			Name:   extra.Name,
			Path:   dest,
			Copied: copied,
		})
	}

	return res, nil
}

func writeFile(dir, name string, data []byte, perm os.FileMode) (File, error) {
	path := filepath.Join(dir, name)
	file := File{Name: name, Path: path}

	_, err := os.Stat(path)
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return file, fmt.Errorf("failed to check existing %s binary: %w", name, err)
	}

	if err := writeFileAtomically(path, data, perm); err != nil {
		return file, fmt.Errorf("failed to write %s binary: %w", name, err)
	}
	file.Copied = true
	return file, nil
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

func copyTree(fsys fs.FS, root, dest string) (copied bool, err error) {
	err = fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
		copied = true
		return nil
	})
	return copied, err
}
