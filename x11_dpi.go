//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// Live DPI tracking for the X11 backend.
//
// X11 has no per-window scale: the desktop publishes one DPI for all
// clients, either through XSETTINGS ("Xft/DPI", what GNOME, Xfce, Cinnamon
// and MATE set, already multiplied by the window scaling factor) or through
// the "Xft.dpi" X resource on the root window (what KDE and `xrdb` set).
// Both live in window properties, so a change in display scale arrives as a
// PropertyNotify. The host listens for it and reloads the font, keeping the
// grid and resizing the window around it, as a font hot-swap does.

const (
	x11DefaultDPI = 96.0
	// xsettingsTypeInteger is the XSETTINGS type tag of a CARD32 value.
	xsettingsTypeInteger = 0
	xsettingsTypeString  = 1
	xsettingsTypeColor   = 2
)

// parseXftDPI returns the Xft.dpi value of an X resource database string
// (the RESOURCE_MANAGER property), or 0 when it has none.
func parseXftDPI(resources string) float64 {
	for _, line := range strings.Split(resources, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != "Xft.dpi" {
			continue
		}
		if dpi, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && dpi > 0 {
			return dpi
		}
	}
	return 0
}

// parseXSettingsDPI returns the Xft/DPI value of an _XSETTINGS_SETTINGS
// property, or 0 when it has none or the data is malformed. The format is
// the freedesktop XSETTINGS one: a byte-order byte, three pad bytes, a
// serial and a setting count, then per setting a type byte, a pad byte, a
// 16-bit name length, the name padded to 4, a 32-bit serial and the value.
// Xft/DPI is an integer holding the DPI times 1024.
func parseXSettingsDPI(data []byte) float64 {
	if len(data) < 12 {
		return 0
	}
	var order binary.ByteOrder = binary.LittleEndian
	if data[0] == 1 { // MSBFirst; LSBFirst is 0
		order = binary.BigEndian
	}
	count := order.Uint32(data[8:12])
	pos := 12
	pad4 := func(n int) int { return (n + 3) &^ 3 }
	for i := uint32(0); i < count; i++ {
		if pos+4 > len(data) {
			return 0
		}
		typ := data[pos]
		nameLen := int(order.Uint16(data[pos+2 : pos+4]))
		pos += 4
		if pos+pad4(nameLen)+4 > len(data) {
			return 0
		}
		name := string(data[pos : pos+nameLen])
		pos += pad4(nameLen) + 4 // name, then the last-change serial
		switch typ {
		case xsettingsTypeInteger:
			if pos+4 > len(data) {
				return 0
			}
			if name == "Xft/DPI" {
				v := int32(order.Uint32(data[pos : pos+4]))
				if v <= 0 {
					return 0
				}
				return float64(v) / 1024
			}
			pos += 4
		case xsettingsTypeString:
			if pos+4 > len(data) {
				return 0
			}
			pos += 4 + pad4(int(order.Uint32(data[pos:pos+4])))
		case xsettingsTypeColor:
			pos += 8
		default:
			return 0
		}
	}
	return 0
}

// x11FontDPI turns a desktop DPI into the font DPI loadBestFont takes and the
// integer scale used for line thickness. The 72 baseline matches the font
// size convention of the other backends (size in pixels at 96 DPI).
func x11FontDPI(dpi float64) (fontDPI float64, scale int) {
	if dpi <= 0 {
		dpi = x11DefaultDPI
	}
	factor := dpi / x11DefaultDPI
	scale = int(factor + 0.5)
	if scale < 1 {
		scale = 1
	}
	return 72.0 * factor, scale
}

// x11DPIWatch is what the host needs to notice a DPI change: the atoms and
// windows whose properties carry the DPI.
type x11DPIWatch struct {
	root            xproto.Window
	resourceManager xproto.Atom
	manager         xproto.Atom
	xsettingsSel    xproto.Atom
	xsettingsProp   xproto.Atom
	// xsettingsOwner is the window of the running XSETTINGS manager, 0 when
	// there is none.
	xsettingsOwner xproto.Window
}

func x11InternAtom(conn *xgb.Conn, name string) xproto.Atom {
	reply, err := xproto.InternAtom(conn, false, uint16(len(name)), name).Reply()
	if err != nil || reply == nil {
		return 0
	}
	return reply.Atom
}

// newX11DPIWatch interns the atoms and finds the XSETTINGS manager of the
// given screen.
func newX11DPIWatch(conn *xgb.Conn, root xproto.Window, screenNum int) *x11DPIWatch {
	w := &x11DPIWatch{
		root:            root,
		resourceManager: x11InternAtom(conn, "RESOURCE_MANAGER"),
		manager:         x11InternAtom(conn, "MANAGER"),
		xsettingsSel:    x11InternAtom(conn, fmt.Sprintf("_XSETTINGS_S%d", screenNum)),
		xsettingsProp:   x11InternAtom(conn, "_XSETTINGS_SETTINGS"),
	}
	w.findXSettingsOwner(conn)
	return w
}

// findXSettingsOwner looks up the XSETTINGS manager window and subscribes to
// changes of its properties.
func (w *x11DPIWatch) findXSettingsOwner(conn *xgb.Conn) {
	w.xsettingsOwner = 0
	if w.xsettingsSel == 0 {
		return
	}
	reply, err := xproto.GetSelectionOwner(conn, w.xsettingsSel).Reply()
	if err != nil || reply == nil || reply.Owner == 0 {
		return
	}
	w.xsettingsOwner = reply.Owner
	xproto.ChangeWindowAttributes(conn, w.xsettingsOwner, xproto.CwEventMask,
		[]uint32{uint32(xproto.EventMaskPropertyChange | xproto.EventMaskStructureNotify)})
}

// subscribe asks for the root window property changes that carry Xft.dpi
// and for the MANAGER announcement of a new XSETTINGS manager.
func (w *x11DPIWatch) subscribe(conn *xgb.Conn) {
	xproto.ChangeWindowAttributes(conn, w.root, xproto.CwEventMask,
		[]uint32{uint32(xproto.EventMaskPropertyChange | xproto.EventMaskStructureNotify)})
}

// readDPI returns the desktop DPI: XSETTINGS first, the Xft.dpi resource
// next, 96 when neither says.
func (w *x11DPIWatch) readDPI(conn *xgb.Conn) float64 {
	if w.xsettingsOwner != 0 && w.xsettingsProp != 0 {
		reply, err := xproto.GetProperty(conn, false, w.xsettingsOwner, w.xsettingsProp, xproto.AtomAny, 0, 1<<20).Reply()
		if err == nil && reply != nil && reply.Format == 8 {
			if dpi := parseXSettingsDPI(reply.Value); dpi > 0 {
				return dpi
			}
		}
	}
	if w.resourceManager != 0 {
		reply, err := xproto.GetProperty(conn, false, w.root, w.resourceManager, xproto.AtomAny, 0, 1<<20).Reply()
		if err == nil && reply != nil && reply.Format == 8 {
			if dpi := parseXftDPI(string(reply.Value)); dpi > 0 {
				return dpi
			}
		}
	}
	return x11DefaultDPI
}

// isDPIProperty reports whether a property change may have changed the DPI.
func (w *x11DPIWatch) isDPIProperty(win xproto.Window, atom xproto.Atom) bool {
	if win == w.root && atom == w.resourceManager && atom != 0 {
		return true
	}
	return win != 0 && win == w.xsettingsOwner && atom == w.xsettingsProp && atom != 0
}

// isNewXSettingsManager reports whether a client message on the root window
// announces a new XSETTINGS manager for this screen.
func (w *x11DPIWatch) isNewXSettingsManager(e *xproto.ClientMessageEvent) bool {
	return e.Window == w.root && e.Type == w.manager && w.manager != 0 &&
		e.Format == 32 && xproto.Atom(e.Data.Data32[1]) == w.xsettingsSel
}

// applyDPILocked switches the host to a desktop DPI: it reloads the font at
// the matching font DPI and hands the new cell size and line scale on. It
// returns false, and changes nothing, when the font DPI is the one the
// window already uses. The grid is left alone. The caller holds h.mu.
func (h *X11Host) applyDPILocked(dpi float64) bool {
	fontDPI, scale := x11FontDPI(dpi)
	if math.Abs(fontDPI-h.dpi) < 0.5 {
		return false
	}
	DebugLog("X11: DPI %.1f -> font DPI %.1f (was %.1f), scale %d", dpi, fontDPI, h.dpi, scale)
	h.dpi = fontDPI
	h.scale = scale
	h.applyFontLocked(h.fontName, h.fontSize)
	return true
}

// refreshDPI re-reads the desktop DPI and, if it changed, rescales the
// window. Runs on the event loop goroutine.
func (h *X11Host) refreshDPI() {
	if h.dpiWatch == nil {
		return
	}
	dpi := h.dpiWatch.readDPI(h.conn)
	h.mu.Lock()
	changed := h.applyDPILocked(dpi)
	h.mu.Unlock()
	if changed {
		h.resizeToGrid()
	}
}

// resizeToGrid asks the X server for a window that holds the current grid
// at the current cell size, and repaints: the ConfigureNotify that follows
// a size change keeps cols x rows, and a same-size change sends none.
func (h *X11Host) resizeToGrid() {
	h.mu.Lock()
	conn, wid := h.conn, h.wid
	cols, rows, cellW, cellH := h.cols, h.rows, h.cellW, h.cellH
	h.mu.Unlock()

	if conn != nil && cols > 0 && rows > 0 && cellW > 0 && cellH > 0 {
		// #nosec G115 -- cols/rows are the terminal's grid size and
		// cellW/cellH are font-metric pixel sizes: small non-negative values.
		width, height := uint32(cols*cellW), uint32(rows*cellH)
		xproto.ConfigureWindow(conn, wid, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{width, height})
	}
	if FrameManager != nil {
		FrameManager.HardRefresh()
	}
}

// cellSize returns the current cell size under the lock: a DPI change or a
// font hot-swap can replace it while input is being translated.
func (h *X11Host) cellSize() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cellW, h.cellH
}

// handleConfigure applies a ConfigureNotify of the host's window: the grid
// follows the new pixel size. It reports whether the grid size changed.
func (h *X11Host) handleConfigure(w, ht uint16) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w == h.width && ht == h.height {
		return false
	}
	h.width, h.height = w, ht
	if h.cellW > 0 && h.cellH > 0 {
		h.cols, h.rows = int(w)/h.cellW, int(ht)/h.cellH
	}
	return true
}
