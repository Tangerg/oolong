package grid

// frameBuffer owns the state every terminal renderer needs to move from one
// frame to the next: the surface believed to be visible, the surface being drawn,
// and the colour depth used when cells cross the wire. Screen and Inline differ
// in how they publish a frame, not in how these three facts are maintained.
type frameBuffer struct {
	front, back *Surface
	depth       Depth
}

func newFrameBuffer(w, h int) frameBuffer {
	return frameBuffer{front: NewSurface(w, h), back: NewSurface(w, h)}
}

func (b *frameBuffer) size() (w, h int) { return b.back.Size() }

// resize changes both surfaces as one operation and reports whether they changed.
// Validation happens before either surface is touched, so recovering a dimension
// panic cannot leave a renderer with mismatched buffers.
func (b *frameBuffer) resize(w, h int) bool {
	w, h, _ = surfaceSize(w, h)
	if cw, ch := b.back.Size(); cw == w && ch == h {
		return false
	}
	b.front.Resize(w, h)
	b.back.Resize(w, h)
	return true
}

func (b *frameBuffer) setDepth(depth Depth) bool {
	if b.depth == depth {
		return false
	}
	b.depth = depth
	return true
}

func (b *frameBuffer) setGround(ground Ground) {
	b.front.SetGround(ground)
	b.back.SetGround(ground)
}

func (b *frameBuffer) begin(cursor *Cursor) View {
	b.back.Reset()
	*cursor = Cursor{}
	view := b.back.View()
	view.cursor = cursor
	return view
}

func (b *frameBuffer) swap() { b.front, b.back = b.back, b.front }
