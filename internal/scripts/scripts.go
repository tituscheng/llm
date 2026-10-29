// Package scripts embeds an install.sh recipe for compiling each inference tool
// from its latest upstream source.
package scripts

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

//go:embed */install.sh
var files embed.FS

const scriptName = "install.sh"

// Names returns tool names that have an install script.
func Names() []string {
	matches, err := fs.Glob(files, "*/"+scriptName)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, path.Dir(match))
	}
	return names
}

// Has reports whether name has an install script.
func Has(name string) bool {
	_, err := File(name)
	return err == nil
}

// File returns the install.sh contents for the named tool.
func File(name string) ([]byte, error) {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	data, err := files.ReadFile(path.Join(name, scriptName))
	if err != nil {
		return nil, fmt.Errorf("unknown tool %q", name)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("install script %q is empty", name)
	}
	return data, nil
}
