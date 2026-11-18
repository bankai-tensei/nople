package trash

import (
	"errors"
	"github.com/bankai-tensei/nople/internal/i18n"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// shFileOpStruct is SHFILEOPSTRUCTW.
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom, pTo            *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

var procSHFileOperationW = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// Move moves the file or folder at path to the Recycle Bin, from where
// the user can bring it back.
func Move(path string) error {
	const (
		foDelete          = 3
		fofSilent         = 0x0004
		fofNoConfirm      = 0x0010
		fofAllowUndo      = 0x0040
		fofNoErrorUI      = 0x0400
		fofNoConfirmMkdir = 0x0200
	)
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return err
	}
	from = append(from, 0) // the list ends with two NULs
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirm | fofSilent | fofNoErrorUI | fofNoConfirmMkdir,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	if r != 0 {
		return errors.New(i18n.T("trash.failed", r))
	}
	if op.fAnyOperationsAborted != 0 {
		return errors.New(i18n.T("trash.aborted"))
	}
	return nil
}
