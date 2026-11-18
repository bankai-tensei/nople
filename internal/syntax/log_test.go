package syntax

import "testing"

func TestLogLine_TimeLevelAndStrings(t *testing.T) {
	cases := []struct {
		line string
		at   []int
		want []Kind
	}{
		{`2026-11-18T12:00:00.123Z ERROR db: query "select 1" failed`, []int{0, 25, 42, 53}, []Kind{KindComment, KindDeleted, KindString, KindNone}},
		{`[12:00:01] WARN disk almost full`, []int{1, 11, 16}, []Kind{KindComment, KindNumber, KindNone}},
		{`Nov 18 12:00:02 host sshd[1]: INFO accepted`, []int{0, 30}, []Kind{KindComment, KindType}},
		{`time=2026-11-18 level=debug msg="cache hit"`, []int{22, 32}, []Kind{KindComment, KindString}},
		{`plain text with an error inside`, []int{19}, []Kind{KindNone}},
	}
	for _, c := range cases {
		h := &Highlight{Lines: [][]Span{LogLine(c.line)}}
		for k, col := range c.at {
			if got := kindsAt(h, 0, col); got != c.want[k] {
				t.Errorf("%q col %d: Kind %d, want %d (%v)", c.line, col, got, c.want[k], h.Lines[0])
			}
		}
	}
}

func TestIsLogPath(t *testing.T) {
	for path, want := range map[string]bool{
		"app.log": true, "APP.LOG": true, "app.log.1": true, "server.log.2026-11-18": true,
		"blog.md": false, "catalog.go": false, "log": false,
	} {
		if got := IsLogPath(path); got != want {
			t.Errorf("IsLogPath(%q) = %v", path, got)
		}
	}
}
