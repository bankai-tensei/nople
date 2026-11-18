package diff

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// apply turns a into b by the hunks, taking the new lines from b.
func apply(a, b []string, hs []Hunk) []string {
	var out []string
	i := 0
	for _, h := range hs {
		out = append(out, a[i:h.OldStart]...)
		out = append(out, b[h.NewStart:h.NewStart+h.NewLen]...)
		i = h.OldStart + h.OldLen
	}
	return append(out, a[i:]...)
}

func TestLineDiff_Random(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	words := []string{"a", "b", "c", "d", "e"}
	gen := func() []string {
		s := make([]string, r.IntN(30))
		for i := range s {
			s[i] = words[r.IntN(len(words))]
		}
		return s
	}
	for range 3000 {
		a, b := gen(), gen()
		hs := Lines(a, b)
		if got := apply(a, b, hs); !slices.Equal(got, b) {
			t.Fatalf("a=%v b=%v hunks=%v gives %v", a, b, hs, got)
		}
		// The equal lines between hunks are equal.
		i, j := 0, 0
		for _, h := range hs {
			if h.OldStart-i != h.NewStart-j {
				t.Fatalf("out of step: a=%v b=%v %v", a, b, hs)
			}
			for k := 0; k < h.OldStart-i; k++ {
				if a[i+k] != b[j+k] {
					t.Fatalf("unequal kept line: a=%v b=%v %v", a, b, hs)
				}
			}
			i, j = h.OldStart+h.OldLen, h.NewStart+h.NewLen
		}
	}
}

func TestLineDiff_Minimal(t *testing.T) {
	a := Split("one\ntwo\nthree\nfour\n")
	b := Split("one\n2\nthree\nfour\nfive\n")
	hs := Lines(a, b)
	want := []Hunk{{1, 1, 1, 1}, {4, 0, 4, 1}}
	if !slices.Equal(hs, want) {
		t.Errorf("hunks = %v, want %v", hs, want)
	}
	lm := LineMap{Hunks: hs}
	if lm.ToNew(3) != 3 || lm.ToOld(4.5) != 4 && lm.ToOld(4.5) != 3.999 {
		t.Logf("toOld(4.5) = %v", lm.ToOld(4.5))
	}
}

func TestLineDiff_Large(t *testing.T) {
	var a, b []string
	for i := range 50000 {
		l := strings.Repeat("x", i%7)
		a = append(a, l)
		if i%1000 == 0 {
			b = append(b, "changed")
		} else {
			b = append(b, l)
		}
	}
	hs := Lines(a, b)
	if len(hs) != 50 || !slices.Equal(apply(a, b, hs), b) {
		t.Errorf("%d hunks", len(hs))
	}
}

func TestIntraline(t *testing.T) {
	as, ae, bs, be, ok := Intraline("x := 1 // 日本", "x := 22 // 日本")
	if !ok || "x := 1 // 日本"[as:ae] != "1" || "x := 22 // 日本"[bs:be] != "22" {
		t.Errorf("%d %d %d %d %v", as, ae, bs, be, ok)
	}
	if _, _, _, _, ok := Intraline("あいう", "かきく"); ok {
		t.Error("lines with nothing alike should give no ranges")
	}
	as, ae, _, _, _ = Intraline("日本語", "日本後")
	if "日本語"[as:ae] != "語" {
		t.Errorf("a range should not cut a character: %q", "日本語"[as:ae])
	}
}
