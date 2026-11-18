//go:build !windows

package app

import "time"

// processStart is not known here; the measure starts when the package does.
func processStart() (time.Time, bool) { return time.Time{}, false }
