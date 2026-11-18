package bigfile

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bankai-tensei/gowaku/widgets/editor"
)

// TestMeasure measures opening large files, for docs/PERFORMANCE.md. It runs
// only when NOPLE_MEASURE lists the sizes to measure, in MB:
//
//	NOPLE_MEASURE=100,1024,10240  the sizes of the logs, written once
//	NOPLE_MEASURE_DIR=dir         where they are written (default: a temp dir)
//	NOPLE_MEASURE_RUNS=5          runs of each size; the median is reported
//	NOPLE_MEASURE_DROP=command    a command run before each run, through sh
//	                              on Linux, to drop the page cache
//
// A run opens the file as Nople does, waits for the whole index, and reads
// the last line: the time Open takes to return (what shows the start of the
// file), the time until the whole file is indexed, the time to read the last
// line once indexed (what a jump to the end costs), and the memory of the
// index, from the heap of Go before and after.
func TestMeasure(t *testing.T) {
	sizes := os.Getenv("NOPLE_MEASURE")
	if sizes == "" {
		t.Skip("set NOPLE_MEASURE to measure")
	}
	dir := os.Getenv("NOPLE_MEASURE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	runs, _ := strconv.Atoi(os.Getenv("NOPLE_MEASURE_RUNS"))
	runs = max(runs, 1)
	for _, s := range strings.Split(sizes, ",") {
		mb, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, fmt.Sprintf("measure-%d.log", mb))
		lines := writeLog(t, path, int64(mb)<<20)
		var open, index, last, mem []float64
		for range runs {
			if cmd := os.Getenv("NOPLE_MEASURE_DROP"); cmd != "" {
				if out, err := exec.Command("sh", "-c", cmd).CombinedOutput(); err != nil {
					t.Fatalf("%s: %v %s", cmd, err, out)
				}
			}
			o, i, l, m := measureOpen(t, path, lines)
			open, index, last, mem = append(open, o), append(index, i), append(last, l), append(mem, m)
		}
		t.Logf("%s %d MB, %d lines, %d runs: open %.1f ms, whole index %.0f ms, last line %.2f ms, index %.1f MB (medians; open runs %v)",
			runtime.GOOS, mb, lines, runs, median(open), median(index), median(last), median(mem), rounded(open))
	}
}

// writeLog writes a log of size bytes at path, unless one is there, and
// returns its number of lines.
func writeLog(t *testing.T, path string, size int64) int {
	line := func(i int) string {
		return fmt.Sprintf("2026-11-18T12:%02d:%02d.%03dZ INFO request %09d \"GET /items/%d\" handled in %d ms\n", i/60%60, i%60, i%1000, i, i%977, i%89)
	}
	if st, err := os.Stat(path); err == nil && st.Size() >= size && st.Size() < size+200 {
		n := 0
		f, _ := os.Open(path)
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			n++
		}
		f.Close()
		return n + 1
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	n := 0
	for written := int64(0); written < size; n++ {
		k, _ := w.WriteString(line(n))
		written += int64(k)
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return n + 1
}

// measureOpen opens path as Nople does and returns the milliseconds Open
// took, those until the whole index, those to read the last line, and the
// MB the index holds.
func measureOpen(t *testing.T, path string, lines int) (float64, float64, float64, float64) {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	u := newUI()
	var ctl editor.Controller
	start := time.Now()
	d, err := Open(&ctl, path, Options{Post: u.post})
	if err != nil {
		t.Fatal(err)
	}
	open := time.Since(start)
	_ = ctl.Line(0)
	deadline := time.After(10 * time.Minute)
	for d.Loading() {
		select {
		case fn := <-u:
			fn()
		case <-deadline:
			t.Fatal("the index never ended")
		}
	}
	index := time.Since(start)
	if ctl.LineCount() != lines {
		t.Fatalf("%d lines, want %d", ctl.LineCount(), lines)
	}
	at := time.Now()
	_ = ctl.Line(ctl.LineCount() - 2)
	last := time.Since(at)
	runtime.GC()
	runtime.ReadMemStats(&after)
	mem := float64(int64(after.HeapInuse)-int64(before.HeapInuse)) / (1 << 20)
	runtime.KeepAlive(&ctl)
	d.Close()
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	return ms(open), ms(index), ms(last), mem
}

func median(v []float64) float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func rounded(v []float64) []string {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = strconv.FormatFloat(x, 'f', 1, 64)
	}
	return out
}
