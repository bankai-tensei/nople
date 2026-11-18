// Command doctypes prints the CFBundleDocumentTypes of Nople.app, which
// packaging/macos/build.sh puts into Info.plist, and writes the icons they
// name into a folder, the Resources of the app:
//
//	go run ./packaging/macos/doctypes -icons Nople.app/Contents/Resources
//
// The types are folders and text, and each extension of
// shellreg.Extensions, the ones Nople offers to open on Windows. Finder then
// offers Nople in "Open With" for them, and lets the user make it the app
// that opens them. Each extension has an entry of its own with the icon of
// its kind, which fileicon draws as it does for Explorer; without one,
// Finder shows the icon of the app on the files that Nople opens. Folders
// and text go by their types, with the icon of a file of no known kind:
// LaunchServices reads the extensions of an entry only when it has no types.
// The rank is Alternate, so that Nople takes no file from the app that opens
// it now until the user says so.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bankai-tensei/nople/internal/fileicon"
	"github.com/bankai-tensei/nople/internal/shellreg"
)

var types = []string{
	"public.folder",
	"public.text",
	"public.plain-text",
	"public.source-code",
	"public.script",
	"public.shell-script",
	"public.json",
	"public.xml",
	"public.yaml",
	"public.log",
	"public.comma-separated-values-text",
	"public.tab-separated-values-text",
	"net.daringfireball.markdown",
}

func main() {
	icons := flag.String("icons", "", "the folder to write the icons of the types into")
	flag.Parse()
	if *icons == "" {
		log.Fatal("doctypes: -icons is needed")
	}
	// writeIcon writes the icon of ext and returns its name in Resources.
	writeIcon := func(ext string) string {
		name := "file.icns"
		if ext != "" {
			name = "file-" + strings.TrimPrefix(ext, ".") + ".icns"
		}
		if err := os.WriteFile(filepath.Join(*icons, name), fileicon.MacIcon(ext), 0o644); err != nil {
			log.Fatal(err)
		}
		return name
	}

	var b strings.Builder
	b.WriteString("\t<key>CFBundleDocumentTypes</key>\n\t<array>\n")
	entry := func(name, icon, key string, values []string) {
		b.WriteString("\t\t<dict>\n")
		fmt.Fprintf(&b, "\t\t\t<key>CFBundleTypeName</key>\n\t\t\t<string>%s</string>\n", name)
		fmt.Fprintf(&b, "\t\t\t<key>CFBundleTypeIconFile</key>\n\t\t\t<string>%s</string>\n", icon)
		b.WriteString("\t\t\t<key>CFBundleTypeRole</key>\n\t\t\t<string>Editor</string>\n")
		b.WriteString("\t\t\t<key>LSHandlerRank</key>\n\t\t\t<string>Alternate</string>\n")
		fmt.Fprintf(&b, "\t\t\t<key>%s</key>\n\t\t\t<array>\n", key)
		for _, v := range values {
			fmt.Fprintf(&b, "\t\t\t\t<string>%s</string>\n", v)
		}
		b.WriteString("\t\t\t</array>\n\t\t</dict>\n")
	}
	entry("Folders and text", writeIcon(""), "LSItemContentTypes", types)
	for _, ext := range shellreg.Extensions {
		e := strings.TrimPrefix(ext, ".")
		entry(strings.ToUpper(e)+" file", writeIcon(ext), "CFBundleTypeExtensions", []string{e})
	}
	b.WriteString("\t</array>\n")
	fmt.Print(b.String())
}
