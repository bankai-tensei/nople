package bigfile

import "os"

// holdOpen says whether a File stays open while it is shown. On Windows it
// does not: Windows does not let a file be renamed over while a program holds
// it open, even with FILE_SHARE_DELETE, so another program that saves by
// writing a new file and renaming it would fail with Access is denied. The
// file is opened for each read there, and checked to be the same file.
const holdOpen = false

// dup is not used where holdOpen is false.
func dup(f *os.File) (*os.File, error) { return os.Open(f.Name()) }
