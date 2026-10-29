package grok

import "strings"

const (
	thinkOpen  = "<think>"
	thinkClose = "</think>"
)

// thinkFilter drops a <think> trace from assistant text.
// Tags may arrive split across chunks. A trailing prefix of a tag is held
// until the next chunk shows whether it is a tag.
type thinkFilter struct {
	inThink bool
	hold    string
}

func (f *thinkFilter) push(chunk string) string {
	buf := f.hold + chunk
	f.hold = ""
	var out strings.Builder
	for len(buf) > 0 {
		if f.inThink {
			end := strings.Index(buf, thinkClose)
			if end >= 0 {
				buf = buf[end+len(thinkClose):]
				f.inThink = false
				continue
			}
			f.hold = tagSuffix(buf, thinkClose)
			return out.String()
		}
		start := strings.Index(buf, thinkOpen)
		if start >= 0 {
			out.WriteString(buf[:start])
			buf = buf[start+len(thinkOpen):]
			f.inThink = true
			continue
		}
		hold := tagSuffix(buf, thinkOpen)
		out.WriteString(buf[:len(buf)-len(hold)])
		f.hold = hold
		return out.String()
	}
	return out.String()
}

// flush emits a held prefix that was not a tag. An unclosed think trace is dropped.
func (f *thinkFilter) flush() string {
	if f.inThink {
		f.hold = ""
		f.inThink = false
		return ""
	}
	text := f.hold
	f.hold = ""
	return text
}

func stripThink(s string) string {
	var filter thinkFilter
	return filter.push(s) + filter.flush()
}

// tagSuffix is the longest suffix of s that is a proper prefix of tag.
func tagSuffix(s, tag string) string {
	maxLen := len(tag) - 1
	if maxLen > len(s) {
		maxLen = len(s)
	}
	for n := maxLen; n > 0; n-- {
		if strings.HasPrefix(tag, s[len(s)-n:]) {
			return s[len(s)-n:]
		}
	}
	return ""
}
