package input

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"
)

// Source represents a single input source with its type detected.
type Source struct {
	Type SourceType
	Data string
}

// SourceType indicates what kind of input source this is.
type SourceType int

const (
	SourceText SourceType = iota
	SourceFile
	SourceDir
	SourceClipboard
	SourceStdin
)

// ReadAuto detects and reads from multiple sources based on their content.
// Arguments can be:
//   - File paths (existing files)
//   - Directory paths (existing directories)
//   - "clipboard:" to read from clipboard
//   - Plain text (any other string)
//
// All sources are concatenated with newlines.
func ReadAuto(args []string) (string, error) {
	return ReadAutoWithPrefix(args, "")
}

// ReadAutoWithPrefix is like ReadAuto but prepends a prefix to each non-stdin source.
func ReadAutoWithPrefix(args []string, prefix string) (string, error) {
	if len(args) == 0 {
		return readStdin(), nil
	}

	var parts []string
	for _, arg := range args {
		source, err := detectSource(arg)
		if err != nil {
			return "", err
		}

		data, err := readSource(source)
		if err != nil {
			return "", err
		}

		if data == "" {
			continue
		}

		if prefix != "" && source.Type != SourceStdin {
			data = prefix + data
		}
		parts = append(parts, data)
	}

	return strings.Join(parts, "\n"), nil
}

// detectSource determines the type of input source from an argument.
func detectSource(arg string) (Source, error) {
	// Check for special clipboard keyword
	if strings.HasPrefix(arg, "clipboard:") || arg == ":clip" || arg == ":paste" {
		return Source{Type: SourceClipboard}, nil
	}

	// Check if it's an existing file
	if info, err := os.Stat(arg); err == nil {
		if info.IsDir() {
			return Source{Type: SourceDir, Data: arg}, nil
		}
		return Source{Type: SourceFile, Data: arg}, nil
	}

	// Otherwise treat as plain text
	return Source{Type: SourceText, Data: arg}, nil
}

// readSource reads content from a single source.
func readSource(source Source) (string, error) {
	switch source.Type {
	case SourceText:
		return source.Data, nil
	case SourceFile:
		data, err := os.ReadFile(source.Data)
		if err != nil {
			return "", err
		}
		return string(data), nil
	case SourceDir:
		return readDir(source.Data)
	case SourceClipboard:
		return clipboard.ReadAll()
	case SourceStdin:
		return readStdin(), nil
	default:
		return "", nil
	}
}

// readDir reads all files in a directory (non-recursively).
func readDir(dirPath string) (string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", err
	}

	var parts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip files that can't be read
		}
		parts = append(parts, string(data))
	}

	return strings.Join(parts, "\n"), nil
}

// readStdin reads all content from standard input.
func readStdin() string {
	scanner := bufio.NewScanner(os.Stdin)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}
