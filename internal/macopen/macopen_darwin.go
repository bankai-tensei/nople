package macopen

import (
	"log"
	"net/url"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// The codes of the Apple Event that opens documents, from AE/AppleEvents.h.
const (
	kCoreEventClass  = 0x61657674 // 'aevt'
	kAEOpenDocuments = 0x6F646F63 // 'odoc'
	keyDirectObject  = 0x2D2D2D2D // '----'
	typeFileURL      = 0x6675726C // 'furl'
)

// handler is kept, as AppKit holds the observer and the Apple Event handler
// without retaining them.
var handler objc.ID

// Listen calls open with the paths of each Apple Event that asks Nople to
// open documents, on the main thread. It must run on the main thread before
// gowaku.Run, which finishes launching the app: the event that starts the
// app comes then, and open may be called before Run has a window.
//
// AppKit puts in its own handler of the event while it finishes launching,
// which hands the files to the delegate of the application, and gowaku's
// delegate has nothing to do with them. So Listen waits for
// NSApplicationWillFinishLaunchingNotification, which AppKit posts after
// that, and puts its handler in place of AppKit's then.
func Listen(open func(paths []string)) {
	sel := objc.RegisterName
	cls, err := objc.RegisterClass("NopleOpenHandler", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: sel("willFinishLaunching:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			objc.ID(objc.GetClass("NSAppleEventManager")).Send(sel("sharedAppleEventManager")).
				Send(sel("setEventHandler:andSelector:forEventClass:andEventID:"),
					self, sel("openDocuments:withReplyEvent:"), uint32(kCoreEventClass), uint32(kAEOpenDocuments))
		}},
		{Cmd: sel("openDocuments:withReplyEvent:"), Fn: func(_ objc.ID, _ objc.SEL, event, _ objc.ID) {
			if paths := pathsOf(event); len(paths) > 0 {
				open(paths)
			}
		}},
	})
	if err != nil {
		log.Printf("nople: files from Finder will not open: %v", err)
		return
	}
	handler = objc.ID(cls).Send(sel("new"))
	objc.ID(objc.GetClass("NSNotificationCenter")).Send(sel("defaultCenter")).
		Send(sel("addObserver:selector:name:object:"),
			handler, sel("willFinishLaunching:"), nsString("NSApplicationWillFinishLaunchingNotification"), objc.ID(0))
}

// pathsOf reads the paths of the files of an open documents event: a list of
// file URLs.
func pathsOf(event objc.ID) []string {
	sel := objc.RegisterName
	list := event.Send(sel("paramDescriptorForKeyword:"), uint32(keyDirectObject))
	if list == 0 {
		return nil
	}
	var paths []string
	n := objc.Send[int64](list, sel("numberOfItems"))
	for i := int64(1); i <= n; i++ { // descriptors count from 1
		d := list.Send(sel("descriptorAtIndex:"), i).Send(sel("coerceToDescriptorType:"), uint32(typeFileURL))
		if d == 0 {
			continue
		}
		data := d.Send(sel("data"))
		size := objc.Send[uint64](data, sel("length"))
		p := objc.Send[unsafe.Pointer](data, sel("bytes"))
		if size == 0 || p == nil {
			continue
		}
		u, err := url.Parse(string(unsafe.Slice((*byte)(p), size)))
		if err != nil || u.Scheme != "file" || u.Path == "" {
			continue
		}
		paths = append(paths, u.Path)
	}
	return paths
}

// nsString makes an NSString of s. Listen runs before any autorelease pool,
// so the string is not autoreleased; the one it makes lives as long as the
// app.
func nsString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("alloc")).
		Send(objc.RegisterName("initWithBytes:length:encoding:"),
			unsafe.Pointer(unsafe.StringData(s)), uint64(len(s)), uint64(4)) // NSUTF8StringEncoding
}
