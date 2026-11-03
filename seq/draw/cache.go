package draw

import (
	"gioui.org/layout"
	"gioui.org/op"
)

// Cache keeps a drawing from one frame to the next, so that a frame that
// only moves it, a pan or a scroll, replays the same ops: the paths and
// labels aren't built again, and the GPU keeps the paths it made from
// them, which it only does for ops kept between frames, and only while
// they are moved rather than scaled.
type Cache struct {
	ops  op.Ops
	call op.CallOp
	dims layout.Dimensions
	key  any
	ok   bool
}

// Layout adds the drawing w makes to gtx: the one kept, when it was made
// for the same key, or else a new one, kept for key. key must be
// comparable, and tell apart everything the drawing depends on.
func (c *Cache) Layout(gtx layout.Context, key any, w layout.Widget) layout.Dimensions {
	if !c.ok || c.key != key {
		c.ops.Reset()
		inner := gtx
		inner.Ops = &c.ops
		rec := op.Record(&c.ops)
		c.dims = w(inner)
		c.call = rec.Stop()
		c.key, c.ok = key, true
	}
	c.call.Add(gtx.Ops)
	return c.dims
}
