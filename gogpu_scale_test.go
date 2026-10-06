//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import "testing"

// The first frame only records the scale; later frames report a change once
// per actual change, and an unreadable scale is ignored.
func TestGogpuHost_NoteScale(t *testing.T) {
	h := &GogpuHost{}
	steps := []struct {
		scale float64
		want  bool
	}{
		{2, false},      // first frame
		{2, false},      // unchanged
		{1, true},       // moved to a 100% display
		{1, false},      // unchanged
		{0, false},      // unreadable, ignored
		{1, false},      // still 1 after the unreadable frame
		{1.5, true},     // fractional scale
		{1.5004, false}, // jitter below the threshold
	}
	for i, s := range steps {
		if got := h.noteScale(s.scale, 800, 600, int(800*s.scale), int(600*s.scale)); got != s.want {
			t.Errorf("step %d: noteScale(%v) = %v, want %v", i, s.scale, got, s.want)
		}
	}
}

// A scale change drops what was rasterised at the old scale but keeps the
// font and the (logical) cell size.
func TestGogpuRenderer_DropScaledCaches(t *testing.T) {
	r := &GogpuRenderer{cellW: 9, cellH: 18}
	r.glyphMemo = map[rune]glyphMemoEntry{'─': {}}
	r.gfxKnown = true

	r.dropScaledCaches()

	if r.glyphMemo != nil {
		t.Error("glyphMemo was not dropped")
	}
	if r.gfxKnown {
		t.Error("gfxKnown was not reset")
	}
	if r.cellW != 9 || r.cellH != 18 {
		t.Errorf("cell size changed to %dx%d, want 9x18", r.cellW, r.cellH)
	}
}
