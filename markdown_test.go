package vtui

import "testing"

func TestParseMarkdownTopic_Heading(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "# Title\nBody text.")
	if topic.Name != "Doc" {
		t.Fatalf("Name = %q, want %q", topic.Name, "Doc")
	}
	wantLines := []string{"#Title#", "Body text."}
	if len(topic.Lines) != len(wantLines) {
		t.Fatalf("Lines = %#v, want %#v", topic.Lines, wantLines)
	}
	for i, want := range wantLines {
		if topic.Lines[i] != want {
			t.Errorf("Lines[%d] = %q, want %q", i, topic.Lines[i], want)
		}
	}
}

func TestParseMarkdownTopic_HeadingInsertsBlankLineAfterContent(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "Intro.\n## Section")
	want := []string{"Intro.", "", "#Section#"}
	if len(topic.Lines) != len(want) {
		t.Fatalf("Lines = %#v, want %#v", topic.Lines, want)
	}
	for i := range want {
		if topic.Lines[i] != want[i] {
			t.Errorf("Lines[%d] = %q, want %q", i, topic.Lines[i], want[i])
		}
	}
}

func TestParseMarkdownTopic_HeadingClosingHashesStripped(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "### Title ###")
	if got, want := topic.Lines[0], "#Title#"; got != want {
		t.Errorf("Lines[0] = %q, want %q", got, want)
	}
}

func TestParseMarkdownTopic_Bold(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "This is **important** and __also this__.")
	want := "This is #important# and #also this#."
	if topic.Lines[0] != want {
		t.Errorf("Lines[0] = %q, want %q", topic.Lines[0], want)
	}
}

func TestParseMarkdownTopic_Link(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "See [the docs](https://example.com/docs) for more.")
	want := "See ~the docs~https://example.com/docs@ for more."
	if topic.Lines[0] != want {
		t.Fatalf("Lines[0] = %q, want %q", topic.Lines[0], want)
	}
	if len(topic.Links) != 1 {
		t.Fatalf("Links = %#v, want exactly one", topic.Links)
	}
	link := topic.Links[0]
	if link.Text != "the docs" || link.Target != "https://example.com/docs" {
		t.Errorf("link = %+v, want Text=%q Target=%q", link, "the docs", "https://example.com/docs")
	}
	if link.Line != 0 {
		t.Errorf("link.Line = %d, want 0", link.Line)
	}
}

func TestParseMarkdownTopic_MultipleLinesTrackLineIndex(t *testing.T) {
	topic := ParseMarkdownTopic("Doc", "# Title\n\n[one](https://a.example)\n[two](https://b.example)")
	var targets []string
	var lines []int
	for _, l := range topic.Links {
		targets = append(targets, l.Target)
		lines = append(lines, l.Line)
	}
	if len(targets) != 2 || targets[0] != "https://a.example" || targets[1] != "https://b.example" {
		t.Fatalf("targets = %#v", targets)
	}
	if lines[0] == lines[1] {
		t.Errorf("both links reported the same line index %d", lines[0])
	}
}

func TestParseMarkdownTopic_LiteralMarkupCharsInHeadingAreStripped(t *testing.T) {
	// A literal '#' or '~' in the source must not survive into the output:
	// it would otherwise be misread as a bold toggle or a link delimiter by
	// help_wrap.go/help_view.go's re-parse of the line at render time.
	topic := ParseMarkdownTopic("Doc", "# C# and ~tilde~ in a heading")
	for _, forbidden := range []string{"#", "~"} {
		line := topic.Lines[0]
		count := 0
		for _, r := range line {
			if string(r) == forbidden {
				count++
			}
		}
		// Exactly the two wrapping delimiters of the heading itself for '#',
		// zero for the stray '~'.
		if forbidden == "#" && count != 2 {
			t.Errorf("heading line %q has %d '#' characters, want exactly 2 (the wrapper)", line, count)
		}
		if forbidden == "~" && count != 0 {
			t.Errorf("heading line %q has a stray '~', want none", line)
		}
	}
}

func TestNewMarkdownView_ShowsParsedContent(t *testing.T) {
	view := NewMarkdownView("Readme", "# Hello\n\nSee [more](https://example.com).")
	if view == nil {
		t.Fatal("NewMarkdownView returned nil")
	}
	if view.current == nil {
		t.Fatal("view has no current topic")
	}
	if view.current.Name != "Readme" {
		t.Errorf("current topic Name = %q, want %q", view.current.Name, "Readme")
	}
	if len(view.current.Links) != 1 || view.current.Links[0].Target != "https://example.com" {
		t.Errorf("current topic Links = %#v", view.current.Links)
	}
}
