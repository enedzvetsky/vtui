//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

// x11XKBTranslator implements keytrans.Translator by talking to the X
// server's XKB extension directly, through github.com/unxed/xkb-go/x11
// (NewKeymapFromX11Device), on vtui's own already-open X11 connection.
//
// This is the primary keyboard path on the X11 backend: it builds the
// keymap straight from the server's XKB GetMap/GetNames/GetControls
// replies, without CGO, libxkbcommon, or shelling out to xkbcomp. If
// building it fails for any reason -- no XKB extension, a malformed wire
// reply, and so on -- vtui falls back to keytrans.NewX11Translator, which
// has its own broader backend chain (libxkbcommon, XIM, purexkb,
// dynamicxkb, xkbcomp, corex11). See newX11Translator below and vtui#10.
import (
	"context"
	"fmt"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/winkeys"
	xkb "github.com/unxed/xkb-go"
	"github.com/unxed/xkb-go/x11"
)

type x11XKBTranslator struct {
	conn      *xgb.Conn
	xkbOpcode byte
	state     *xkb.State
}

// newX11KeyboardTranslator builds a keyboard translator directly from the
// X server's XKB extension via github.com/unxed/xkb-go/x11, reusing conn
// (vtui's existing xgb connection for the window) rather than opening a
// second one. It returns an error describing why when that isn't
// possible, so the caller can fall back to keytrans.NewX11Translator.
func newX11KeyboardTranslator(conn *xgb.Conn) (*x11XKBTranslator, error) {
	if conn == nil {
		return nil, fmt.Errorf("xkb-x11: no X11 connection")
	}

	// x11.NewKeymapFromX11Device registers the XKEYBOARD extension on conn
	// itself, but doesn't hand back the major opcode it found -- and
	// TranslateX11 below needs it for the live XkbGetState request that
	// keeps modifier/group state in sync between key events. Registering
	// the extension a second time is harmless: X servers answer redundant
	// QueryExtension calls the same way every time. keytrans's own
	// purexkb backend (backend_purexkb.go in github.com/unxed/keytrans)
	// does the same thing for the same reason.
	major, err := xkbMajorOpcode(conn)
	if err != nil {
		return nil, err
	}

	ctx := xkb.NewContext(context.Background(), xkb.ContextNoFlags)
	km, err := x11.NewKeymapFromX11Device(ctx, conn, x11.UseCoreKbd)
	if err != nil {
		return nil, fmt.Errorf("xkb-x11: %w", err)
	}

	return &x11XKBTranslator{
		conn:      conn,
		xkbOpcode: major,
		state:     km.NewState(),
	}, nil
}

// xkbMajorOpcode registers the XKEYBOARD extension on conn (if not
// already registered) and returns its major opcode.
func xkbMajorOpcode(conn *xgb.Conn) (byte, error) {
	reply, err := xproto.QueryExtension(conn, uint16(len("XKEYBOARD")), "XKEYBOARD").Reply()
	if err != nil {
		return 0, fmt.Errorf("xkb-x11: QueryExtension(XKEYBOARD): %w", err)
	}
	if reply == nil || !reply.Present {
		return 0, fmt.Errorf("xkb-x11: X server does not support the XKEYBOARD extension")
	}
	return reply.MajorOpcode, nil
}

// Name implements keytrans.Translator.
func (t *x11XKBTranslator) Name() string {
	return "xkb-x11"
}

func (t *x11XKBTranslator) translateKeysym(detail uint8, isDown bool) winkeys.InputEvent {
	kc := xkb.Keycode(detail)
	sym := t.state.KeyGetOneSym(kc)
	char := t.state.KeyGetUTF32(kc)
	vk := keysymToVK(uint32(sym))

	if vk == 0 {
		// Some keysyms (e.g. layout-specific letters keysymToVK doesn't
		// know) only resolve to a VK once modifiers are stripped, because
		// keysymToVK matches on the unshifted Latin keysym. Recompute the
		// symbol with an unmodified mask, purely to find a VK, then
		// restore the real state. Mirrors keytrans's purexkb backend.
		bm, lam, lom := t.state.BaseMods(), t.state.LatchedMods(), t.state.LockedMods()
		bg, lag, lkg := t.state.BaseGroup(), t.state.LatchedGroup(), t.state.LockedGroup()

		t.state.UpdateMask(0, 0, 0, 0, 0, 0)
		vkSym := t.state.KeyGetOneSym(kc)
		vk = keysymToVK(uint32(vkSym))

		t.state.UpdateMask(bm, lam, lom, bg, lag, lkg)
	}

	return winkeys.InputEvent{
		Type:           winkeys.KeyEventType,
		VirtualKeyCode: vk,
		Char:           char,
		KeyDown:        isDown,
		RepeatCount:    1,
	}
}

// TranslateX11 implements keytrans.Translator.
func (t *x11XKBTranslator) TranslateX11(detail uint8, state uint16, isDown bool) winkeys.InputEvent {
	// Sync modifier/group state with the server before translating: the
	// core X11 KeyPress/KeyRelease "state" field alone doesn't reliably
	// carry the active XKB group, so ask the server directly via the raw
	// XKB GetState request (minor opcode 4). jezek/xgb has no XKB
	// extension package to generate this from, so it's built by hand --
	// the same approach keytrans's purexkb backend uses.
	if t.conn != nil {
		buf := make([]byte, 8)
		buf[0] = t.xkbOpcode
		buf[1] = 4 // XkbGetState
		// #nosec G115 -- buf is the fixed 8-byte GetState request above, len(buf)/4 is always 2
		xgb.Put16(buf[2:], uint16(len(buf)/4))
		xgb.Put16(buf[4:], uint16(x11.UseCoreKbd))

		cookie := t.conn.NewCookie(true, true)
		t.conn.NewRequest(buf, cookie)
		if reply, err := cookie.Reply(); err == nil && len(reply) >= 18 {
			// #nosec G115 -- XKB limits keyboards to 4 groups (indices 0-3), so the
			// baseGroup/latchedGroup wire values below always fit in xkb.Group's uint8.
			baseGroup := xkb.Group(xgb.Get16(reply[14:]))
			// #nosec G115 -- see baseGroup above.
			latchedGroup := xkb.Group(xgb.Get16(reply[16:]))
			t.state.UpdateMask(
				xkb.ModMask(reply[9]),
				xkb.ModMask(reply[10]),
				xkb.ModMask(reply[11]),
				baseGroup,
				latchedGroup,
				xkb.Group(reply[13]),
			)
		}
	}

	event := t.translateKeysym(detail, isDown)
	event.InputSource = "xkb-x11"
	return event
}

// TranslateWayland implements keytrans.Translator. vtui never selects this
// translator on the Wayland backend (see newX11Translator, only called
// from x11_host.go), but it's implemented the same way keytrans's
// purexkb backend does for interface completeness.
func (t *x11XKBTranslator) TranslateWayland(keycode uint32, isDown bool) winkeys.InputEvent {
	// #nosec G115 -- evdev keycodes fit in a byte once the XKB +8 offset is applied;
	// keytrans's purexkb backend relies on the same range for the same conversion.
	event := t.translateKeysym(uint8(keycode+8), isDown)
	event.InputSource = "xkb-x11"
	return event
}

// UpdateWaylandModifiers implements keytrans.Translator.
func (t *x11XKBTranslator) UpdateWaylandModifiers(modsDepressed, modsLatched, modsLocked, group uint32) {
	// #nosec G115 -- XKB limits keyboards to 4 groups (indices 0-3), well within uint8.
	waylandGroup := xkb.Group(group)
	t.state.UpdateMask(xkb.ModMask(modsDepressed), xkb.ModMask(modsLatched), xkb.ModMask(modsLocked), 0, 0, waylandGroup)
}

// Close implements keytrans.Translator.
func (t *x11XKBTranslator) Close() {}
