package vtui

import (
	"bytes"
	"strings"
	"testing"
)

// roundedButtonEnv selects a graphical backend and the given glyph style for
// one test and restores both afterwards.
func roundedButtonEnv(t *testing.T, backend string, style GlyphStyle) {
	t.Helper()
	prevStyle := CurrentGlyphStyle()
	prevBackend := ActiveBackend()
	SetGlyphStyle(style)
	SetActiveBackend(backend)
	t.Cleanup(func() {
		SetGlyphStyle(prevStyle)
		SetActiveBackend(prevBackend)
		SetDefaultPalette()
	})
}

func drawButton(t *testing.T, text string, mutate func(b *Button)) (*ScreenBuf, *Button) {
	t.Helper()
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 1)
	b := NewButton(1, 0, text)
	if mutate != nil {
		mutate(b)
	}
	b.Show(scr)
	return scr, b
}

// padRow pads a dump row to the 20 columns the test screen has.
func padRow(s string) string { return s + strings.Repeat(" ", 20-len([]rune(s))) }

// previewRow returns the text row of the screen dump, the form the Ctrl+Shift+P
// dump shows a button in.
func previewRow(scr *ScreenBuf, y int) string {
	var buf bytes.Buffer
	scr.Dump(&buf)
	lines := strings.Split(buf.String(), "\n")
	return lines[2+y]
}

func TestButtonRounded_TextTerminalKeepsClassicLook(t *testing.T) {
	// The style is process-wide: a plain terminal (no graphical backend)
	// must keep the brackets even when the application selected "rounded".
	roundedButtonEnv(t, "", GlyphStyleRounded)
	scr, _ := drawButton(t, "OK", nil)
	if got := previewRow(scr, 0); got != padRow(" [ OK ]") {
		t.Errorf("text terminal dump = %q, want the classic button", got)
	}
	checkCell(t, scr, 3, 0, 'O', Palette[ColDialogButton])
}

func TestButtonRounded_ClassicStyleOnGraphicalBackendKeepsClassicLook(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleClassic)
	scr, _ := drawButton(t, "OK", nil)
	if got := previewRow(scr, 0); got != padRow(" [ OK ]") {
		t.Errorf("classic dump = %q", got)
	}
	checkCell(t, scr, 3, 0, 'O', Palette[ColDialogButton])
}

func TestButtonRounded_DrawsTintedUnderlinedBlockWithoutBrackets(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	scr, b := drawButton(t, "OK", nil)

	// Same width as the classic "[ OK ]": 2 + 2 + 2 cells.
	if got := previewRow(scr, 0); got != padRow("   OK") {
		t.Fatalf("rounded dump = %q, want the caption without brackets", got)
	}
	if w := b.X2 - b.X1 + 1; w != 6 {
		t.Fatalf("button width = %d, want 6 (layout must not change)", w)
	}

	base := Palette[ColDialogButton]
	wantBg := roundedButtonBack(attrColorRGB(base, true))
	for x := 1; x <= 6; x++ {
		a := scr.GetCell(x, 0).Attributes
		if a&IsBgRGB == 0 || GetRGBBack(a) != wantBg {
			t.Errorf("cell %d background = %06X, want %06X", x, GetRGBBack(a), wantBg)
		}
		underlined := a&CommonLvbUnderscore != 0
		if want := x == 3 || x == 4; underlined != want {
			t.Errorf("cell %d underlined = %v, want %v (only the caption)", x, underlined, want)
		}
		if got := contrastRatioRGB(attrColorRGB(a, false), attrColorRGB(a, true)); x == 3 && got < roundedButtonMinContrast {
			t.Errorf("caption contrast = %.2f, want >= %.1f", got, roundedButtonMinContrast)
		}
	}
	if wantBg == attrColorRGB(base, true) {
		t.Error("the button background was not tinted")
	}
}

func TestButtonRounded_TintDirection(t *testing.T) {
	cases := []struct {
		name string
		bg   uint32
		want uint32
	}{
		{"dark theme gets 20% lighter", 0x000000, 0x333333},
		{"dark blue gets 20% lighter", 0x0000A0, 0x3333B3},
		{"light grey gets 25% darker", 0xAAAAAA, 0x808080},
		{"white gets 25% darker", 0xFFFFFF, 0xBFBFBF},
	}
	for _, c := range cases {
		if got := roundedButtonBack(c.bg); got != c.want {
			t.Errorf("%s: roundedButtonBack(%06X) = %06X, want %06X", c.name, c.bg, got, c.want)
		}
	}
}

func TestButtonRounded_CaptionContrastIsCorrected(t *testing.T) {
	// Grey on grey is unreadable once the block is tinted: the caption must
	// be pushed towards black on a light block and towards white on a dark one.
	for _, c := range []struct{ fg, bg uint32 }{
		{0x909090, 0xAAAAAA},
		{0x404040, 0x000000},
		{0x808080, 0x808080},
		{0xFF00FF, 0x00FF00},
	} {
		attr := SetRGBBoth(0, c.fg, c.bg)
		out := roundedButtonAttr(attr, false, true)
		got := contrastRatioRGB(attrColorRGB(out, false), attrColorRGB(out, true))
		if got < roundedButtonMinContrast {
			t.Errorf("fg %06X on bg %06X: contrast %.2f after correction, want >= %.1f", c.fg, c.bg, got, roundedButtonMinContrast)
		}
	}

	// A caption that is readable already is left alone.
	attr := SetRGBBoth(0, 0xFFFFFF, 0x000000)
	if out := roundedButtonAttr(attr, false, false); GetRGBFore(out) != 0xFFFFFF {
		t.Errorf("readable caption changed to %06X", GetRGBFore(out))
	}
}

func TestButtonRounded_DisabledKeepsFadedCaption(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	scr, _ := drawButton(t, "OK", func(b *Button) { b.SetDisabled(true) })
	a := scr.GetCell(3, 0).Attributes
	got := contrastRatioRGB(attrColorRGB(a, false), attrColorRGB(a, true))
	if got < roundedButtonDisabledContrast {
		t.Errorf("disabled caption contrast = %.2f, want >= %.1f", got, roundedButtonDisabledContrast)
	}
	if a&CommonLvbUnderscore == 0 {
		t.Error("a disabled button keeps its underlined caption")
	}
}

func TestButtonRounded_StatesStayDistinguishable(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	normal, _ := drawButton(t, "OK", nil)
	focused, _ := drawButton(t, "OK", func(b *Button) { b.SetFocus(true) })
	def, _ := drawButton(t, "OK", func(b *Button) { b.IsDefault = true })
	n := normal.GetCell(3, 0).Attributes
	f := focused.GetCell(3, 0).Attributes
	d := def.GetCell(3, 0).Attributes
	if n == f || n == d {
		t.Errorf("normal %016X, focused %016X, default %016X: states must differ", n, f, d)
	}
}

func TestButtonRounded_HotkeyKeepsItsAttribute(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	scr, _ := drawButton(t, "Sa&ve", nil)
	if got := previewRow(scr, 0); got != padRow("   Save") {
		t.Fatalf("rounded dump = %q, want the caption without brackets or ampersand", got)
	}
	hot := scr.GetCell(5, 0).Attributes // the 'v'
	plain := scr.GetCell(4, 0).Attributes
	if hot == plain {
		t.Error("the hotkey letter lost its highlight")
	}
	if hot&CommonLvbUnderscore == 0 {
		t.Error("the hotkey letter is part of the underlined caption")
	}
}

func TestButtonRounded_RawShortTextStillRendersAsIs(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)
	b := NewButton(0, 0, "placeholder")
	b.ScreenObject.SetText("[←]")
	b.Show(scr)
	if got := ScreenRow(scr, 0, 0, 2); got != "[←]" {
		t.Errorf("raw text = %q, want it drawn as is", got)
	}
}
