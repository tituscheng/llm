package serve

import "strings"

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// thinkScan separates <think> traces from assistant text.
// Tags may arrive split across chunks. A trailing prefix of a tag is held
// until the next chunk shows whether it is a tag.
type thinkScan struct {
	inThink    bool
	dropLead   bool
	sawContent bool
	hold       string
}

func (s *thinkScan) push(in string) (string, string) {
	s.hold += in
	var content, reasoning strings.Builder
	for {
		if s.inThink {
			if s.drainThink(&reasoning) {
				return content.String(), reasoning.String()
			}
			continue
		}
		if s.drainText(&content) {
			return content.String(), reasoning.String()
		}
	}
}

func (s *thinkScan) flush() (string, string) {
	if s.inThink {
		text := strings.TrimRight(s.finishThink(s.hold), "\r\n")
		s.hold = ""
		s.inThink = false
		s.dropLead = false
		return "", text
	}
	text := s.hold
	s.hold = ""
	if text != "" {
		s.sawContent = true
	}
	return text, ""
}

// drainThink reports whether more input is required.
func (s *thinkScan) drainThink(reasoning *strings.Builder) bool {
	idx := strings.Index(s.hold, thinkClose)
	if idx >= 0 {
		piece := strings.TrimRight(s.finishThink(s.hold[:idx]), "\r\n")
		writeTrace(reasoning, piece)
		s.hold = s.hold[idx+len(thinkClose):]
		s.inThink = false
		s.dropLead = false
		if !s.sawContent {
			s.hold = strings.TrimLeft(s.hold, "\r\n")
		}
		return false
	}
	keep := tagSuffix(s.hold, thinkClose)
	emit := s.hold[:len(s.hold)-keep]
	s.hold = s.hold[len(s.hold)-keep:]
	writeTrace(reasoning, s.finishThink(emit))
	return true
}

// drainText reports whether more input is required.
func (s *thinkScan) drainText(content *strings.Builder) bool {
	idx := strings.IndexByte(s.hold, '<')
	if idx < 0 {
		s.emitContent(content, s.hold)
		s.hold = ""
		return true
	}
	s.emitContent(content, s.hold[:idx])
	rest := s.hold[idx:]
	if strings.HasPrefix(rest, thinkOpen) {
		s.hold = rest[len(thinkOpen):]
		s.inThink = true
		s.dropLead = true
		return false
	}
	if strings.HasPrefix(thinkOpen, rest) {
		s.hold = rest
		return true
	}
	s.emitContent(content, "<")
	s.hold = rest[1:]
	return false
}

func (s *thinkScan) emitContent(content *strings.Builder, text string) {
	if text == "" {
		return
	}
	s.sawContent = true
	content.WriteString(text)
}

func (s *thinkScan) finishThink(text string) string {
	if !s.dropLead {
		return text
	}
	trimmed := strings.TrimLeft(text, "\r\n")
	if trimmed == "" {
		return ""
	}
	s.dropLead = false
	return trimmed
}

func writeTrace(dst *strings.Builder, text string) {
	if text == "" {
		return
	}
	if dst.Len() > 0 {
		dst.WriteByte('\n')
	}
	dst.WriteString(text)
}

func tagSuffix(s, tag string) int {
	maxLen := len(tag) - 1
	if maxLen > len(s) {
		maxLen = len(s)
	}
	for n := maxLen; n > 0; n-- {
		if strings.HasPrefix(tag, s[len(s)-n:]) {
			return n
		}
	}
	return 0
}

func applyThink(content, reasoning string) (string, string) {
	var scan thinkScan
	clean, trace := scan.push(content)
	cleanTail, traceTail := scan.flush()
	if reasoning != "" {
		return clean + cleanTail, reasoning
	}
	return clean + cleanTail, strings.TrimSpace(trace + traceTail)
}
