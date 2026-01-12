package input

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"llm/internal/llm"

	"github.com/atotto/clipboard"
)

// SourceType indicates what kind of input source this is.
type SourceType int

const (
	SourceText SourceType = iota
	SourceFile
	SourceDir
	SourceGit
	SourceClipboard
	SourceStdin
)

// Source represents a single input source with its content and token count.
type Source struct {
	Type       SourceType
	Name       string
	Content    string
	TokenCount int
}

// ReadAuto detects and reads from multiple sources based on their content.
// Arguments can be:
//   - File paths (existing files)
//   - Directory paths (existing directories)
//   - "@git" to read all modified and untracked files from git
//   - "clipboard:" to read from clipboard
//   - Plain text (any other string)
//
// All sources are concatenated with newlines.
func ReadAuto(args []string) ([]Source, error) {
	if len(args) == 0 {
		// Read from stdin
		content := readStdin()
		return []Source{{
			Type:       SourceStdin,
			Name:       "stdin",
			Content:    content,
			TokenCount: llm.CountToken(content),
		}}, nil
	}

	var sources []Source
	for _, arg := range args {
		source, err := detectAndRead(arg)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}

	return sources, nil
}

// detectAndRead detects the source type and reads its content.
func detectAndRead(arg string) (Source, error) {
	// Check for special git keyword
	if arg == "@git" {
		return readGit()
	}

	// Check for special clipboard keyword
	if strings.HasPrefix(arg, "clipboard:") || arg == ":clip" || arg == ":paste" {
		return readClipboard()
	}

	// Check if it's an existing file or directory
	if info, err := os.Stat(arg); err == nil {
		if info.IsDir() {
			return readDir(arg)
		}
		return readFile(arg)
	}

	// Otherwise treat as plain text
	content := arg
	return Source{
		Type:       SourceText,
		Name:       "text",
		Content:    content,
		TokenCount: llm.CountToken(content),
	}, nil
}

// readFile reads a single file.
func readFile(path string) (Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, err
	}
	content := string(data)
	return Source{
		Type:       SourceFile,
		Name:       path,
		Content:    content,
		TokenCount: llm.CountToken(content),
	}, nil
}

// readDir reads all files in a directory (non-recursively).
func readDir(dirPath string) (Source, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return Source{}, err
	}

	var parts []string
	var fileNames []string

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
		fileNames = append(fileNames, entry.Name())
	}

	content := strings.Join(parts, "\n")
	return Source{
		Type:       SourceDir,
		Name:       dirPath,
		Content:    content,
		TokenCount: llm.CountToken(content),
	}, nil
}

// readGit reads all modified and untracked files from git.
func readGit() (Source, error) {
	modified, untracked, err := getGitStatusPaths()
	if err != nil {
		return Source{}, err
	}

	var parts []string
	var fileNames []string

	// Modified files
	for _, path := range modified {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parts = append(parts, string(data))
		fileNames = append(fileNames, path)
	}

	// Untracked files
	for _, path := range untracked {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parts = append(parts, string(data))
		fileNames = append(fileNames, path)
	}

	content := strings.Join(parts, "\n")
	return Source{
		Type:       SourceGit,
		Name:       "@git",
		Content:    content,
		TokenCount: llm.CountToken(content),
	}, nil
}

// readClipboard reads content from clipboard.
func readClipboard() (Source, error) {
	content, err := clipboard.ReadAll()
	if err != nil {
		return Source{}, err
	}
	return Source{
		Type:       SourceClipboard,
		Name:       "clipboard",
		Content:    content,
		TokenCount: llm.CountToken(content),
	}, nil
}

// getGitStatusPaths returns the modified and untracked files from git.
func getGitStatusPaths() ([]string, []string, error) {
	cmd := exec.Command("git", "status", "--porcelain=v2", "-z")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, err
	}

	var modified, untracked []string

	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}
		if i := bytes.IndexByte(data, 0); i >= 0 {
			return i + 1, data[0:i], nil
		}
		if atEOF {
			return len(data), data[0:len(data)], nil
		}
		return 0, nil, nil
	})

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		status := fields[0]
		path := fields[len(fields)-1] // last field is always the path

		switch status {
		case "1", "2", "u": // changed tracked files (modified, renamed, unmerged)
			if !contains(modified, path) {
				modified = append(modified, path)
			}
		case "?": // untracked
			if !contains(untracked, path) {
				untracked = append(untracked, path)
			}
		}
	}

	return modified, untracked, scanner.Err()
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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

// TotalTokens calculates the total token count from a slice of sources.
func TotalTokens(sources []Source) int {
	total := 0
	for _, s := range sources {
		total += s.TokenCount
	}
	return total
}
