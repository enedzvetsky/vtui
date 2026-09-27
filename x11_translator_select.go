//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

// This file selects the keyboard translator vtui uses on the X11 backend:
// try the pure Go xkb-go/x11 path first (x11_xkb_translator.go), and fall
// back to keytrans.NewX11Translator only if that fails (vtui#10). The
// selection logic lives here, factored through x11TranslatorFactories, so
// it can be unit-tested with fakes instead of a live X server (see
// x11_translator_select_test.go).
import (
	"os"

	"github.com/jezek/xgb"
	"github.com/unxed/keytrans"
)

// x11TranslatorFactories names the two translator constructors
// newX11Translator chooses between, so tests can substitute fakes.
type x11TranslatorFactories struct {
	// xkbX11 builds the primary translator from conn. Production code
	// sets this to newX11KeyboardTranslator (wrapped to satisfy the
	// keytrans.Translator interface).
	xkbX11 func(conn *xgb.Conn) (keytrans.Translator, error)
	// keytransFallback builds the fallback translator. Production code
	// sets this to keytrans.NewX11Translator.
	keytransFallback func(info keytrans.OSInfo) keytrans.Translator
}

var defaultX11TranslatorFactories = x11TranslatorFactories{
	xkbX11: func(conn *xgb.Conn) (keytrans.Translator, error) {
		return newX11KeyboardTranslator(conn)
	},
	keytransFallback: keytrans.NewX11Translator,
}

// newX11Translator builds the keyboard translator vtui uses on the X11
// backend, on the given already-open X11 connection.
func newX11Translator(conn *xgb.Conn, windowID uint32) keytrans.Translator {
	return newX11TranslatorWith(defaultX11TranslatorFactories, conn, windowID)
}

// newX11TranslatorWith is newX11Translator with the two candidate
// constructors passed in explicitly, so tests can exercise both branches
// without a real X11 connection.
func newX11TranslatorWith(f x11TranslatorFactories, conn *xgb.Conn, windowID uint32) keytrans.Translator {
	if tr, err := f.xkbX11(conn); err == nil {
		DebugLog("X11: using xkb-go/x11 keyboard translator (backend: %s)", tr.Name())
		return tr
	} else {
		DebugLog("X11: xkb-go/x11 keyboard translator unavailable (%v), falling back to keytrans", err)
	}

	info := keytrans.OSInfo{
		DisplayString: os.Getenv("DISPLAY"),
		XgbConn:       conn,
		WindowID:      windowID,
	}
	return f.keytransFallback(info)
}
