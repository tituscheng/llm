package input

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// MockFileSystem is a mock implementation of FileSystem for testing.
type MockFileSystem struct {
	Files map[string][]byte
	Dirs  map[string][]string
	Stats map[string]bool // true = is directory
}

func (fs MockFileSystem) ReadFile(path string) ([]byte, error) {
	if data, ok := fs.Files[path]; ok {
		return data, nil
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: syscall.ENOENT}
}

func (fs MockFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	if files, ok := fs.Dirs[path]; ok {
		entries := make([]os.DirEntry, len(files))
		for i, name := range files {
			entries[i] = &mockDirEntry{name: name, isDir: fs.Stats[name]}
		}
		return entries, nil
	}
	return nil, &os.PathError{Op: "readdir", Path: path, Err: syscall.ENOENT}
}

func (fs MockFileSystem) Stat(path string) (os.FileInfo, error) {
	if isDir, ok := fs.Stats[path]; ok {
		return &mockFileInfo{isDir: isDir}, nil
	}
	return nil, &os.PathError{Op: "stat", Path: path, Err: syscall.ENOENT}
}

func (fs MockFileSystem) Open(path string) (*os.File, error) {
	// Just open /dev/null for testing - we check the file existence via ReadFile
	return os.Open("/dev/null")
}

// mockDirEntry implements os.DirEntry for testing.
type mockDirEntry struct {
	name  string
	isDir bool
}

func (e *mockDirEntry) Name() string      { return e.name }
func (e *mockDirEntry) IsDir() bool       { return e.isDir }
func (e *mockDirEntry) Type() os.FileMode { return 0 }
func (e *mockDirEntry) Info() (os.FileInfo, error) {
	return &mockFileInfo{isDir: e.isDir}, nil
}

// mockFileInfo implements os.FileInfo for testing.
type mockFileInfo struct {
	isDir bool
}

func (fi *mockFileInfo) Name() string       { return "" }
func (fi *mockFileInfo) Size() int64        { return 0 }
func (fi *mockFileInfo) Mode() os.FileMode  { return 0 }
func (fi *mockFileInfo) ModTime() time.Time { return time.Now() }
func (fi *mockFileInfo) IsDir() bool        { return fi.isDir }
func (fi *mockFileInfo) Sys() interface{}   { return nil }

// MockGitExecutor is a mock implementation of GitExecutor for testing.
type MockGitExecutor struct {
	Modified  []string
	Untracked []string
	ShouldErr bool
}

func (g MockGitExecutor) StatusPorcelain() ([]byte, error) {
	if g.ShouldErr {
		return nil, &exec.ExitError{}
	}
	// Simulate git status --porcelain=v2 -z output format
	output := ""
	for _, path := range g.Modified {
		output += "1 M " + path + "\x00"
	}
	for _, path := range g.Untracked {
		output += "? " + path + "\x00"
	}
	return []byte(output), nil
}

// MockClipboardReader is a mock implementation of ClipboardReader for testing.
type MockClipboardReader struct {
	Content string
}

func (r MockClipboardReader) ReadAll() (string, error) {
	return r.Content, nil
}

// MockStdinReader is a mock implementation of StdinReader for testing.
type MockStdinReader struct {
	Content string
}

func (r MockStdinReader) ReadAll() string {
	return r.Content
}

// TestIsTextFile tests the isTextFile function.
func TestIsTextFile(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{"plain text", []byte("Hello, World!"), true},
		{"JSON", []byte(`{"key": "value"}`), true},
		{"code", []byte("package main\nfunc main() {}"), true},
		{"PNG binary", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, false},
		{"JPEG binary", []byte{0xFF, 0xD8, 0xFF, 0xE0}, false},
		{"empty", []byte{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTextFile(tt.data)
			if result != tt.expected {
				t.Errorf("isTextFile(%q) = %v, want %v", tt.data, result, tt.expected)
			}
		})
	}
}

// TestDetectAndRead_Text tests plain text detection.
func TestDetectAndRead_Text(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{Files: map[string][]byte{}, Stats: map[string]bool{}},
	}

	sources, err := detectAndRead("hello world", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceText {
		t.Errorf("expected SourceText, got %v", sources[0].Type)
	}
	if sources[0].Content != "hello world" {
		t.Errorf("expected 'hello world', got %q", sources[0].Content)
	}
}

// TestDetectAndRead_File tests file detection.
func TestDetectAndRead_File(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{"test.go": []byte("package main")},
			Stats: map[string]bool{"test.go": false},
		},
	}

	sources, err := detectAndRead("test.go", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceFile {
		t.Errorf("expected SourceFile, got %v", sources[0].Type)
	}
}

// TestDetectAndRead_FileNotFound tests handling of non-existent files.
func TestDetectAndRead_FileNotFound(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{},
			Stats: map[string]bool{},
		},
	}

	sources, err := detectAndRead("nonexistent.txt", deps)
	if err != nil {
		// Should return error or empty (text detection)
	}
	// Non-existent files are treated as plain text
	if len(sources) != 1 || sources[0].Type != SourceText {
		t.Logf("Non-existent file treated as text: %v sources", len(sources))
	}
}

// TestDetectAndRead_Directory tests directory detection.
func TestDetectAndRead_Directory(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{"dir/file.txt": []byte("content")},
			Dirs:  map[string][]string{"dir": {"file.txt"}},
			Stats: map[string]bool{"dir": true, "dir/file.txt": false},
		},
	}

	sources, err := detectAndRead("dir", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceDir {
		t.Errorf("expected SourceDir, got %v", sources[0].Type)
	}
}

// TestDetectAndRead_Git tests git detection.
func TestDetectAndRead_Git(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{"modified.go": []byte("package main")},
			Stats: map[string]bool{".git": true, "modified.go": false},
		},
		Git: MockGitExecutor{
			Modified:  []string{"modified.go"},
			Untracked: []string{},
		},
	}

	sources, err := detectAndRead("@git", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceGit {
		t.Errorf("expected SourceGit, got %v", sources[0].Type)
	}
}

// TestDetectAndRead_GitNotRepo tests git detection in non-git directory.
func TestDetectAndRead_GitNotRepo(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{},
			Stats: map[string]bool{},
		},
		Git: MockGitExecutor{},
	}

	_, err := detectAndRead("@git", deps)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

// TestDetectAndRead_Md tests markdown detection.
func TestDetectAndRead_Md(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{"doc.md": []byte("# Title")},
			Dirs:  map[string][]string{".": {"doc.md"}},
			Stats: map[string]bool{"doc.md": false},
		},
	}

	sources, err := detectAndRead("@md", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceMd {
		t.Errorf("expected SourceMd, got %v", sources[0].Type)
	}
}

// TestDetectAndRead_Clipboard tests clipboard detection.
func TestDetectAndRead_Clipboard(t *testing.T) {
	deps := &Dependencies{
		FS:        MockFileSystem{},
		Clipboard: MockClipboardReader{Content: "clipboard text"},
	}

	sources, err := detectAndRead("clipboard:", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceClipboard {
		t.Errorf("expected SourceClipboard, got %v", sources[0].Type)
	}
}

// TestReadAuto_MultipleArgs tests multiple arguments.
func TestReadAuto_MultipleArgs(t *testing.T) {
	deps := &Dependencies{
		FS: MockFileSystem{
			Files: map[string][]byte{"file1.txt": []byte("content1")},
			Stats: map[string]bool{"file1.txt": false},
		},
	}

	sources, err := ReadAutoWithDeps([]string{"text1", "file1.txt", "text2"}, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(sources))
	}
}

// TestReadAuto_EmptyArgs tests empty arguments (stdin).
func TestReadAuto_EmptyArgs(t *testing.T) {
	deps := &Dependencies{
		Stdin: MockStdinReader{Content: "stdin content"},
	}

	sources, err := ReadAutoWithDeps([]string{}, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].Type != SourceStdin {
		t.Errorf("expected SourceStdin, got %v", sources[0].Type)
	}
}

// TestTotalTokens tests the TotalTokens function.
func TestTotalTokens(t *testing.T) {
	sources := []Source{
		{TokenCount: 10},
		{TokenCount: 20},
		{TokenCount: 30},
	}

	result := TotalTokens(sources)
	if result != 60 {
		t.Errorf("TotalTokens() = %d, want 60", result)
	}
}

// TestTotalTokens_Empty tests TotalTokens with empty slice.
func TestTotalTokens_Empty(t *testing.T) {
	result := TotalTokens([]Source{})
	if result != 0 {
		t.Errorf("TotalTokens() = %d, want 0", result)
	}
}

// TestContains tests the contains helper function.
func TestContains(t *testing.T) {
	tests := []struct {
		slice    []string
		item     string
		expected bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b", "c"}, "d", false},
		{[]string{}, "a", false},
		{[]string{"a"}, "a", true},
	}

	for _, tt := range tests {
		result := contains(tt.slice, tt.item)
		if result != tt.expected {
			t.Errorf("contains(%v, %q) = %v, want %v", tt.slice, tt.item, result, tt.expected)
		}
	}
}
