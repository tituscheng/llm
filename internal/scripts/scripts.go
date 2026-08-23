// Package scripts embeds a Taskfile for compiling each inference binary
// from its latest upstream source.
package scripts

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed */Taskfile.yaml
var files embed.FS

const taskfileName = "Taskfile.yaml"

// Names returns binary names that have a compile Taskfile.
func Names() []string {
	matches, err := fs.Glob(files, "*/"+taskfileName)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, path.Dir(match))
	}
	return names
}

// Has reports whether name has a compile Taskfile.
func Has(name string) bool {
	_, err := File(name)
	return err == nil
}

// File returns the Taskfile.yaml contents for the named binary.
func File(name string) ([]byte, error) {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("unknown compile script %q", name)
	}
	data, err := files.ReadFile(path.Join(name, taskfileName))
	if err != nil {
		return nil, fmt.Errorf("unknown compile script %q", name)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("compile script %q is empty", name)
	}
	return data, nil
}
