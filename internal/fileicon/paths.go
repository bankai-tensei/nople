package fileicon

import "github.com/bankai-tensei/gowaku/paint"

// iconFile is the page, the icon of a file of no known kind.
var iconFile = filled("M6.5 2H14l6 6v12a2 2 0 0 1-2 2H6.5a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z")

// outline parses the path d and strokes it at the weight of the line icons of
// gowaku.
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
