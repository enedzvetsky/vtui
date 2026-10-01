package vtui

// Rounded-style push buttons (f4 #285).
//
// With the "rounded" GlyphStyle a graphical window draws a Button as a flat,
// underlined caption on a tinted block instead of the classic "[ Caption ]":
// the block is lighter than the palette background of the button's state on a
// dark theme and darker on a light one, the caption's colour is corrected to
// stay readable on it, and the angle brackets are gone. The block keeps the
// 2-cell "ears" the brackets used to occupy (as blank, tinted cells), so the
// button is exactly as wide as before and switching style never moves
// anything in a dialog.
//
// Unlike a checkbox glyph, this cannot be a symbolic token every backend
// resolves for itself: it recolours cells, and the colours are the widget's
// to choose. The widget therefore asks whether it is drawing for a graphical
// backend at all (a text terminal keeps the classic look whatever style the
// application selected, because the style is a process-wide setting).

const (
	// roundedButtonLighten is how far a dark background moves towards white.
	roundedButtonLighten = 0.20
	// roundedButtonDarken is how far a light background moves towards black.
	roundedButtonDarken = 0.25
	// roundedButtonMinContrast is the WCAG contrast ratio the caption keeps
	// against the tinted block (AA for normal text); a disabled button, whose
	// dimmed caption is meant to look faded, keeps roundedButtonDisabledContrast.
	roundedButtonMinContrast      = 4.5
	roundedButtonDisabledContrast = 3.0
	// roundedButtonLightLuminance is the relative luminance above which a
	// background counts as light: black and white text have equal contrast on it.
	roundedButtonLightLuminance = 0.179
)

// roundedButtonsActive reports whether Buttons are drawn in the rounded style:
// a graphical backend owns the screen and the rounded GlyphStyle is selected.
func roundedButtonsActive() bool {
	return CurrentGlyphStyle() == GlyphStyleRounded && ActiveBackend() != ""
}

// attrColorRGB resolves one side of an attribute to a packed 0xRRGGBB value
// the way the graphical renderers do: 24-bit colours as they are, palette
// indices through ThemePalette.
func attrColorRGB(attr uint64, background bool) uint32 {
	if background {
		if attr&IsBgRGB != 0 {
			return GetRGBBack(attr)
		}
		return ThemePalette[GetIndexBack(attr)]
	}
	if attr&IsFgRGB != 0 {
		return GetRGBFore(attr)
	}
	return ThemePalette[GetIndexFore(attr)]
}

// mixRGB moves each channel of from towards to by t (0..1).
func mixRGB(from, to uint32, t float64) uint32 {
	var out uint32
	for _, shift := range [...]uint{16, 8, 0} {
		a := float64((from >> shift) & 0xFF)
		b := float64((to >> shift) & 0xFF)
		out |= uint32(a+(b-a)*t+0.5) << shift
	}
	return out
}

// roundedButtonBack tints a button background: lighter on a dark one, darker
// on a light one.
func roundedButtonBack(bg uint32) uint32 {
	if relativeLuminanceRGB(bg) > roundedButtonLightLuminance {
		return mixRGB(bg, 0x000000, roundedButtonDarken)
	}
	return mixRGB(bg, 0xFFFFFF, roundedButtonLighten)
}

// readableOn returns fg, nudged towards white or black (whichever contrasts
// with bg) just far enough that its contrast ratio against bg reaches minRatio;
// fg itself when it is readable already.
func readableOn(fg, bg uint32, minRatio float64) uint32 {
	if contrastRatioRGB(fg, bg) >= minRatio {
		return fg
	}
	target := uint32(0xFFFFFF)
	if relativeLuminanceRGB(bg) > roundedButtonLightLuminance {
		target = 0x000000
	}
	const steps = 20
	for i := 1; i <= steps; i++ {
		if c := mixRGB(fg, target, float64(i)/steps); contrastRatioRGB(c, bg) >= minRatio {
			return c
		}
	}
	return target
}

// roundedButtonAttr converts a classic button attribute into the rounded one:
// tinted background, caption colour kept readable, caption underlined when
// underline is set (the ears are blank, so they are not).
func roundedButtonAttr(attr uint64, disabled, underline bool) uint64 {
	bg := roundedButtonBack(attrColorRGB(attr, true))
	want := roundedButtonMinContrast
	if disabled {
		want = roundedButtonDisabledContrast
	}
	fg := readableOn(attrColorRGB(attr, false), bg, want)
	out := SetRGBBoth(attr, fg, bg)
	if underline {
		out |= CommonLvbUnderscore
	}
	return out
}

// drawRoundedButton draws the button's label (the text between the classic
// ears) as a tinted block: 2 blank cells, the underlined label, 2 blank cells.
func (b *Button) drawRoundedButton(scr *ScreenBuf, label string, hotkeyPos int, n, h uint64) {
	disabled := b.IsDisabled()
	blankN := roundedButtonAttr(n, disabled, false)
	labelN := roundedButtonAttr(n, disabled, true)
	labelH := roundedButtonAttr(h, disabled, true)

	p := NewPainter(scr)
	blank := []CharInfo{{Char: uint64(' '), Attributes: blankN}, {Char: uint64(' '), Attributes: blankN}}
	scr.Write(b.X1, b.Y1, blank)
	p.DrawHighlightedText(b.X1+2, b.Y1, label, hotkeyPos, labelN, labelH)
	scr.Write(b.X1+2+StringWidth(label), b.Y1, blank)
}
