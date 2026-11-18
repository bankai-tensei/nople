package syntax

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Logs have no lexer in chroma, and a large file is most often one. LexLog
// colors what a reader of a log looks for: the time of each line, dimmed, its
// level, in the color of how bad it is, and the quoted strings.

// IsLogPath reports whether the file at path is a log: one ending in .log,
// or rotated from one, as app.log.1 or app.log.2026-11-18.
func IsLogPath(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(name, ".log") || strings.Contains(name, ".log.")
}

var (
	// logTime is a time near the start of a line: an ISO date and time, a
	// time of day alone, or the date of syslog.
	logTime = regexp.MustCompile(`^[\[(]?(?:\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?|\d{2}:\d{2}:\d{2}(?:[.,]\d+)?|[A-Z][a-z]{2} [ \d]\d \d{2}:\d{2}:\d{2})[\])]?`)
	// logLevel is a level written in capitals, or in small letters after
	// level= or "level":.
	logLevel = regexp.MustCompile(`\b(FATAL|PANIC|CRITICAL|CRIT|ERROR|ERR|SEVERE|WARNING|WARN|NOTICE|INFO|DEBUG|TRACE|VERBOSE)\b|(?:\blevel=|\blvl=|"level":\s*")(fatal|panic|critical|error|err|warning|warn|notice|info|debug|trace)\b`)
	// logString is a string in double quotes, with its escapes.
	logString = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
)

// logReach is how far into a line its time and level are looked for.
const logReach = 160

// levelKind is the kind a level shows as.
func levelKind(level string) Kind {
	switch strings.ToUpper(level) {
	case "FATAL", "PANIC", "CRITICAL", "CRIT", "ERROR", "ERR", "SEVERE":
		return KindDeleted
	case "WARNING", "WARN":
		return KindNumber
	case "NOTICE", "INFO":
		return KindType
	}
	return KindComment
}

// LogLine returns the spans of a line of a log.
func LogLine(s string) []Span {
	var spans []Span
	head := s[:min(len(s), logReach)]
	at := 0
	if m := logTime.FindStringIndex(head); m != nil {
		spans = append(spans, Span{int32(m[0]), int32(m[1]), KindComment})
		at = m[1]
	}
	if m := logLevel.FindStringSubmatchIndex(head[at:]); m != nil {
		g := 2
		if m[2] < 0 {
			g = 4
		}
		start, end := at+m[g], at+m[g+1]
		spans = append(spans, Span{int32(start), int32(end), levelKind(s[start:end])})
	}
	for _, m := range logString.FindAllStringIndex(s, -1) {
		if overlaps(spans, m[0], m[1]) {
			continue
		}
		spans = append(spans, Span{int32(m[0]), int32(m[1]), KindString})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	return spans
}

// overlaps reports whether a span of spans meets the bytes from start to end.
func overlaps(spans []Span, start, end int) bool {
	for _, sp := range spans {
		if int(sp.Start) < end && start < int(sp.End) {
			return true
		}
	}
	return false
}

// LexLog colors text as a log, line by line.
func LexLog(text string) *Highlight {
	lines := strings.Split(text, "\n")
	h := &Highlight{Lines: make([][]Span, len(lines))}
	for i, l := range lines {
		h.Lines[i] = LogLine(strings.TrimSuffix(l, "\r"))
	}
	return h
}
