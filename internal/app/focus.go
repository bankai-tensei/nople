package app

import (
	"github.com/bankai-tensei/gowaku/core"
	"github.com/bankai-tensei/gowaku/layout"
)

// focusWithin tells when the keyboard focus enters or leaves its child, so
// that the application knows which editor the user works in.
type focusWithin struct {
	onChange func(within bool)
	child    core.Widget
}

// Key implements core.Widget.
func (focusWithin) Key() core.Key { return nil }

// ChildWidget implements core.SingleChildWidget.
func (f focusWithin) ChildWidget() core.Widget { return f.child }

// CreateRenderObject implements core.RenderWidget.
func (f focusWithin) CreateRenderObject() core.RenderObject {
	return &renderFocusWithin{onChange: f.onChange}
}

// UpdateRenderObject implements core.RenderWidget.
func (f focusWithin) UpdateRenderObject(ro core.RenderObject) {
	ro.(*renderFocusWithin).onChange = f.onChange
}

type renderFocusWithin struct {
	layout.RenderProxy
	onChange func(within bool)
}

// FocusWithinChanged implements core.FocusWithinListener.
func (r *renderFocusWithin) FocusWithinChanged(within bool) {
	if r.onChange != nil {
		r.onChange(within)
	}
}
