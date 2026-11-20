package app

import (
	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/geom"
	"github.com/bankai-tensei/gowaku/icons"
	"github.com/bankai-tensei/gowaku/paint"
	"github.com/bankai-tensei/gowaku/widgets/basic"

	"github.com/bankai-tensei/nople/internal/fileicon"
)

// The icons, on the grid of 24: the stock icons of gowaku, and outlines of
// width 1.75, their weight, for the ones it has not.
var (
	iconFiles    = icons.Document()
	iconSearch   = icons.Search()
	iconTerminal = outline("M4 5h16v14H4z M7 9l3 3l-3 3 M12 15h5")
	iconSettings = icons.Settings()
	iconOutline  = outline("M4 6h16 M8 12h12 M8 18h12 M4 12h0.01 M4 18h0.01")
	iconBranch   = outline("M6 3v12 M6 15a3 3 0 1 0 0 6a3 3 0 1 0 0-6z M18 3a3 3 0 1 0 0 6a3 3 0 1 0 0-6z M18 9a9 9 0 0 1-9 9")
	iconFolder   = filled("M3 6.5A2.5 2.5 0 0 1 5.5 4H9l2 2h7.5A2.5 2.5 0 0 1 21 8.5v9a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 17.5z")
	iconHome     = icons.Home()
)

// outline parses the path d and strokes it.
func outline(d string) *paint.Path {
	p, err := paint.ParseSVGPath(d)
	if err != nil {
		panic(err)
	}
	return p.Stroke(paint.StrokeStyle{Width: 1.75, Cap: paint.CapRound, Join: paint.JoinRound})
}

// filled parses the path d.
func filled(d string) *paint.Path {
	p, err := paint.ParseSVGPath(d)
	if err != nil {
		panic(err)
	}
	return p
}

// folderColor is the color of the folder icon.
var folderColor = geom.Hex(0xD9A441)

// fileIcon is the icon of the explorer for path, a folder if dir.
func fileIcon(path string, dir bool, plain geom.Color, dark bool) core.Widget {
	if dir {
		return basic.Icon{Path: iconFolder, Size: 16, Color: folderColor}
	}
	return fileicon.Widget(path, plain, geom.Color{}, dark)
}
