package app

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/bankai-tensei/gowaku"
)

// When NOPLE_STARTUP_LOG names a file, Nople measures how fast it starts and
// quits: it appends to the file how long after the process was created it
// drew its first frame, and the frame after it took in what it opened (a
// folder, a file, the session), with the memory Go holds then. It is for the
// numbers the README gives; see docs/PERFORMANCE.md.

// startupTrace is a measure of the start in progress.
type startupTrace struct {
	path  string
	start time.Time
	first time.Duration
	built bool
}

// packageStart is when the package was initialized, the start where the OS
// does not tell when the process was created.
var packageStart = time.Now()

// newStartupTrace returns the measure the environment asks for, or nil.
func newStartupTrace() *startupTrace {
	path := os.Getenv("NOPLE_STARTUP_LOG")
	if path == "" {
		return nil
	}
	start, ok := processStart()
	if !ok {
		start = packageStart
	}
	return &startupTrace{path: path, start: start}
}

// builtOnce notes the first build of the window. What it posts runs once that
// build has been drawn.
func (t *startupTrace) builtOnce(post func(func())) {
	if t == nil || t.built {
		return
	}
	t.built = true
	post(func() { t.first = time.Since(t.start) })
}

// opened is posted after what the window opens at the start; what it posts
// runs once the window has been drawn with it, and ends the measure.
func (t *startupTrace) opened(post func(func())) {
	if t == nil {
		return
	}
	post(func() {
		post(func() {
			ready := time.Since(t.start)
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				fmt.Fprintf(f, "first=%.1fms ready=%.1fms heap=%.1fMB sys=%.1fMB\n",
					ms(t.first), ms(ready), mb(m.HeapInuse), mb(m.Sys))
				f.Close()
			}
			gowaku.Quit()
		})
	})
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
func mb(b uint64) float64        { return float64(b) / (1 << 20) }
