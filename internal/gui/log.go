package gui

import (
	"bytes"
	"sync"
)

const maxLogLines = 2000

type lineWriter struct {
	append func(string)
	mu     sync.Mutex
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	w.buf = bytes.ReplaceAll(w.buf, []byte{'\r'}, []byte{'\n'})
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.append(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 {
		return
	}
	w.append(string(w.buf))
	w.buf = nil
}

func appendCapped(lines []string, line string) []string {
	lines = append(lines, line)
	if len(lines) <= maxLogLines {
		return lines
	}
	return lines[len(lines)-maxLogLines:]
}
