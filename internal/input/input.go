package input

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"llm/internal/llm"

	"github.com/atotto/clipboard"
	"github.com/fatih/color"
	"github.com/gabriel-vasile/mimetype"
)

// Source represents a single input source with its content and token count.
type Source struct {
	Type       SourceType
	Name       string
	Content    string
	TokenCount int
}

// SourceType indicates what kind of input source this is.
type SourceType int

const (
	SourceText SourceType = iota
	SourceFile
	SourceDir
	SourceGit
	SourceMd
	SourceClipboard
	SourceStdin
)

// ReadAuto detects and reads from multiple sources based on their content.
// Arguments can be:
//   - File paths (existing files)
//   - Directory paths (existing directories)
//   - "@git" to read all modified and untracked files from git
//   - "@md" to read all markdown files in the current directory
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
		detected, err := detectAndRead(arg)
		if err != nil {
			color.Yellow("Warning: %v\n", err)
			continue
		}
		sources = append(sources, detected...)
	}

	return sources, nil
}

// detectAndRead detects the source type and reads its content.
// Returns a slice of sources (multiple for @git and @md).
func detectAndRead(arg string) ([]Source, error) {
	// Check for special git keyword
	if arg == "@git" {
		return readGit()
	}

	// Check for special md keyword
	if arg == "@md" {
		return readMd(".")
	}

	// Check for special clipboard keyword
	if strings.HasPrefix(arg, "clipboard:") || arg == ":clip" || arg == ":paste" {
		return readClipboard()
	}

	// Check if it's an existing file or directory
	info, err := os.Stat(arg)
	if err == nil {
		if info.IsDir() {
			return readDir(arg)
		}
		return readFile(arg)
	}

	// Otherwise treat as plain text
	content := arg
	return []Source{{
		Type:       SourceText,
		Name:       "text",
		Content:    content,
		TokenCount: llm.CountToken(content),
	}}, nil
}

// readFile reads a single file.
func readFile(path string) ([]Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", path)
	}

	// Skip binary files using mimetype
	if !isTextFile(data) {
		return nil, nil
	}

	content := string(data)
	return []Source{{
		Type:       SourceFile,
		Name:       path,
		Content:    content,
		TokenCount: llm.CountToken(content),
	}}, nil
}

// readDir reads all files in a directory (non-recursively).
func readDir(dirPath string) ([]Source, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %s", dirPath)
	}

	var sources []Source
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Skip binary files
		filePath := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Skip binary files using mimetype
		if !isTextFile(data) {
			continue
		}

		content := string(data)
		sources = append(sources, Source{
			Type:       SourceDir,
			Name:       entry.Name(),
			Content:    content,
			TokenCount: llm.CountToken(content),
		})
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no readable text files in: %s", dirPath)
	}

	return sources, nil
}

// readGit reads all modified and untracked files from git.
func readGit() ([]Source, error) {
	// Check if we're in a git repository
	if !isGitRepo() {
		return nil, fmt.Errorf("not a git repository (must run from a directory with .git)")
	}

	modified, untracked, err := getGitStatusPaths()
	if err != nil {
		return nil, fmt.Errorf("git command failed: %w", err)
	}

	if len(modified) == 0 && len(untracked) == 0 {
		return nil, fmt.Errorf("no modified or untracked files in git")
	}

	var sources []Source

	// Process modified files
	for _, path := range modified {
		// Skip hidden files
		if strings.HasPrefix(filepath.Base(path), ".") {
			continue
		}
		if !isTextFileByPath(path) {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		content := string(data)
		sources = append(sources, Source{
			Type:       SourceGit,
			Name:       path,
			Content:    content,
			TokenCount: llm.CountToken(content),
		})
	}

	// Process untracked files
	for _, path := range untracked {
		// Skip hidden files
		if strings.HasPrefix(filepath.Base(path), ".") {
			continue
		}
		if !isTextFileByPath(path) {
			continue
		}

		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		content := string(data)
		sources = append(sources, Source{
			Type:       SourceGit,
			Name:       path,
			Content:    content,
			TokenCount: llm.CountToken(content),
		})
	}

	return sources, nil
}

// readMd reads all markdown files in a directory.
func readMd(dirPath string) ([]Source, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found: %s", dirPath)
	}

	var sources []Source
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process markdown files
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}

		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		// Skip binary files using mimetype
		if !isTextFile(data) {
			continue
		}

		content := string(data)
		sources = append(sources, Source{
			Type:       SourceMd,
			Name:       entry.Name(),
			Content:    content,
			TokenCount: llm.CountToken(content),
		})
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no markdown files found in: %s", dirPath)
	}

	return sources, nil
}

// readClipboard reads content from clipboard.
func readClipboard() ([]Source, error) {
	content, err := clipboard.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read clipboard: %w", err)
	}
	return []Source{{
		Type:       SourceClipboard,
		Name:       "clipboard",
		Content:    content,
		TokenCount: llm.CountToken(content),
	}}, nil
}

// isGitRepo checks if the current directory is a git repository.
func isGitRepo() bool {
	_, err := os.Stat(".git")
	return err == nil
}

// isTextFile checks if file content is text (not binary) using mimetype.
func isTextFile(data []byte) bool {
	mime := mimetype.Detect(data)
	mimeStr := mime.String()

	// Exclude common binary types
	excludeMimes := []string{
		"image/", "video/", "audio/",
		"application/pdf",
		"application/zip", "application/x-rar", "application/gzip",
		"application/x-executable", "application/x-sharedlib",
		"application/octet-stream",
	}
	for _, prefix := range excludeMimes {
		if strings.HasPrefix(mimeStr, prefix) {
			return false
		}
	}

	return true
}

// isTextFileByPath checks if a file is text by reading a small portion.
func isTextFileByPath(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	// Read first 512 bytes to detect type
	buf := make([]byte, 512)
	n, _ := file.Read(buf)

	return isTextFile(buf[:n])
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
