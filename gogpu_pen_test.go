//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"testing"

	"github.com/gogpu/gpucontext"
)

func TestPenPointerActionTurnsAPenIntoMouseActions(t *testing.T) {
	pen := func(typ gpucontext.PointerEventType, b gpucontext.Button) gpucontext.PointerEvent {
		return gpucontext.PointerEvent{Type: typ, PointerType: gpucontext.PointerTypePen, Button: b}
	}
	cases := []struct {
		name   string
		ev     gpucontext.PointerEvent
		action penAction
		button gpucontext.MouseButton
		ok     bool
	}{
		{"touch down", pen(gpucontext.PointerDown, gpucontext.ButtonLeft), penPress, gpucontext.MouseButtonLeft, true},
		{"release", pen(gpucontext.PointerUp, gpucontext.ButtonLeft), penRelease, gpucontext.MouseButtonLeft, true},
		{"barrel button", pen(gpucontext.PointerDown, gpucontext.ButtonRight), penPress, gpucontext.MouseButtonRight, true},
		{"middle", pen(gpucontext.PointerDown, gpucontext.ButtonMiddle), penPress, gpucontext.MouseButtonMiddle, true},
		{"hover or drag", pen(gpucontext.PointerMove, gpucontext.ButtonNone), penMove, 0, true},
	}
	for _, c := range cases {
		action, button, ok := penPointerAction(c.ev)
		if ok != c.ok || (ok && (action != c.action || (action != penMove && button != c.button))) {
			t.Errorf("%s: got (%v, %v, %v), want (%v, %v, %v)", c.name, action, button, ok, c.action, c.button, c.ok)
		}
	}

	// A mouse already reaches the mouse callbacks, a finger is left to the
	// operating system's own emulation, and other pen events say nothing to do.
	for _, typ := range []gpucontext.PointerType{gpucontext.PointerTypeMouse, gpucontext.PointerTypeTouch} {
		if _, _, ok := penPointerAction(gpucontext.PointerEvent{Type: gpucontext.PointerDown, PointerType: typ}); ok {
			t.Errorf("pointer type %v must not be forwarded", typ)
		}
	}
	if _, _, ok := penPointerAction(pen(gpucontext.PointerCancel, gpucontext.ButtonNone)); ok {
		t.Error("a cancelled pen event is not a mouse action")
	}
}
