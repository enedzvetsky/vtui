//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"errors"
	"testing"

	"github.com/jezek/xgb"
	"github.com/unxed/keytrans"
	"github.com/unxed/winkeys"
)

// fakeX11Translator is a minimal keytrans.Translator stand-in so the
// selection logic in newX11TranslatorWith can be tested without a real X11
// connection or keymap.
type fakeX11Translator struct {
	name string
}

func (f *fakeX11Translator) Name() string { return f.name }
func (f *fakeX11Translator) TranslateX11(detail uint8, state uint16, isDown bool) winkeys.InputEvent {
	return winkeys.InputEvent{}
}
func (f *fakeX11Translator) TranslateWayland(keycode uint32, isDown bool) winkeys.InputEvent {
	return winkeys.InputEvent{}
}
func (f *fakeX11Translator) UpdateWaylandModifiers(modsDepressed, modsLatched, modsLocked, group uint32) {
}
func (f *fakeX11Translator) Close() {}

func TestNewX11TranslatorWith_PrefersXKBX11WhenItSucceeds(t *testing.T) {
	want := &fakeX11Translator{name: "xkb-x11"}
	keytransCalled := false

	got := newX11TranslatorWith(x11TranslatorFactories{
		xkbX11: func(conn *xgb.Conn) (keytrans.Translator, error) {
			return want, nil
		},
		keytransFallback: func(info keytrans.OSInfo) keytrans.Translator {
			keytransCalled = true
			return &fakeX11Translator{name: "should-not-be-used"}
		},
	}, nil, 0)

	if got != keytrans.Translator(want) {
		t.Errorf("expected the xkb-go/x11 translator to be returned, got %v", got)
	}
	if keytransCalled {
		t.Error("keytrans fallback must not be invoked when the xkb-go/x11 path succeeds")
	}
}

func TestNewX11TranslatorWith_FallsBackToKeytransOnFailure(t *testing.T) {
	want := &fakeX11Translator{name: "keytrans-fallback"}
	var gotDisplay string
	var gotWindowID uint32

	got := newX11TranslatorWith(x11TranslatorFactories{
		xkbX11: func(conn *xgb.Conn) (keytrans.Translator, error) {
			return nil, errors.New("xkb-x11: X server does not support the XKEYBOARD extension")
		},
		keytransFallback: func(info keytrans.OSInfo) keytrans.Translator {
			gotDisplay = info.DisplayString
			gotWindowID = info.WindowID
			return want
		},
	}, nil, 42)

	if got != keytrans.Translator(want) {
		t.Errorf("expected the keytrans fallback translator to be returned, got %v", got)
	}
	if gotWindowID != 42 {
		t.Errorf("expected the fallback to receive windowID 42, got %d", gotWindowID)
	}
	_ = gotDisplay // set from the environment; not asserted here
}

func TestNewX11TranslatorWith_FallbackCanAlsoFail(t *testing.T) {
	got := newX11TranslatorWith(x11TranslatorFactories{
		xkbX11: func(conn *xgb.Conn) (keytrans.Translator, error) {
			return nil, errors.New("xkb-x11: no X11 connection")
		},
		keytransFallback: func(info keytrans.OSInfo) keytrans.Translator {
			return nil
		},
	}, nil, 0)

	if got != nil {
		t.Errorf("expected nil when both the xkb-go/x11 path and the keytrans fallback fail, got %v", got)
	}
}
