package i18n

import "github.com/ebitengine/purego"

// osUILanguages are the preferred languages of macOS (System Settings >
// General > Language & Region), best first, such as "ja-JP". An app started
// from Finder or the Dock has no LANG, so without these it would show
// English whatever the language of the Mac. They come from CoreFoundation
// through purego, as Nople builds without cgo.
func osUILanguages() []string {
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	var (
		copyPreferredLanguages func() uintptr
		arrayGetCount          func(uintptr) int
		arrayGetValueAtIndex   func(uintptr, int) uintptr
		stringGetCString       func(uintptr, *byte, int, uint32) bool
		release                func(uintptr)
	)
	purego.RegisterLibFunc(&copyPreferredLanguages, cf, "CFLocaleCopyPreferredLanguages")
	purego.RegisterLibFunc(&arrayGetCount, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&arrayGetValueAtIndex, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&stringGetCString, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&release, cf, "CFRelease")

	const utf8 = 0x08000100 // kCFStringEncodingUTF8
	langs := copyPreferredLanguages()
	if langs == 0 {
		return nil
	}
	defer release(langs)
	var out []string
	buf := make([]byte, 64)
	for i := range arrayGetCount(langs) {
		if s := arrayGetValueAtIndex(langs, i); s != 0 && stringGetCString(s, &buf[0], len(buf), utf8) {
			n := 0
			for n < len(buf) && buf[n] != 0 {
				n++
			}
			out = append(out, string(buf[:n]))
		}
	}
	return out
}
