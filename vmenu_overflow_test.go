package vtui

import "testing"

// TestVMenu_LongItemStaysInsideTheBox (f4 #1706): an item wider than the menu
// is cut with an ellipsis inside the box; nothing is painted over the border,
// the scrollbar column or the cells to the right of the menu.
func TestVMenu_LongItemStaysInsideTheBox(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(60, 6)
	m := NewVMenu("")
	m.AddItem(MenuItem{Text: "BigBlue TerminalPlus Nerd Font Complete Mono Windows Compatible"})
	m.AddItem(MenuItem{Text: "&Short"})
	m.SetPosition(0, 0, 20, 4)
	m.Show(scr)

	// The first row: " " then the text cut to the box, ending in the ellipsis
	// right before the right border (column 20).
	if got := scr.GetCell(19, 1).Char; got != '…' {
		t.Errorf("the last text cell of the long item = %q, want an ellipsis", rune(got))
	}
	for x := 21; x < 60; x++ {
		if c := scr.GetCell(x, 1).Char; c != 0 && c != ' ' {
			t.Fatalf("cell %d beside the menu was painted with %q", x, rune(c))
		}
	}
	// A short item is drawn as before, with its accent letter intact.
	if got := scr.GetCell(2, 2).Char; got != 'S' {
		t.Errorf("the short item starts with %q, want S", rune(got))
	}
}
