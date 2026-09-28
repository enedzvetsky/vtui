package vtui

import (
	"regexp"
	"strings"
)

// f4#258 asked for a Markdown browser, and the owner's decision was to base
// it on the existing help engine rather than write a new renderer: HelpTopic
// (help_engine.go) is already a generic "lines of text plus links" document,
// and HelpView/help_wrap.go already know how to scroll, word-wrap and
// navigate one, regardless of where it came from. This file is the first
// step of generalizing that pair into a reusable vtui component: a Markdown
// parser that targets the SAME markup HelpTopic's own author-facing format
// uses (`#bold#`, `~text~target@`) rather than inventing a second rendering
// path, so wrapping, coloring and link navigation all keep working with zero
// changes to help_view.go/help_wrap.go.
//
// This first part deliberately covers only what maps cleanly onto that
// existing markup:
//   - ATX headings (# .. ######) become a bold line;
//   - **bold**/__bold__ spans become #bold#;
//   - [text](target) links become ~text~target@, so Enter/click on one
//     works exactly like any other help link -- including HelpView's
//     existing http(s):// handling, which opens the target in the system
//     browser today. Fetching the target and rendering it in place instead
//     (f4#258's part with an HTTPS-backed showcase) is separate, later work.
//
// Not covered yet, and left as future work rather than guessed at: italics,
// nested emphasis, lists/tables reformatted beyond passing their text
// through unchanged, and escaping a literal '#', '~' or '@' in the source
// (the same authoring constraint HelpTopic's own native format already has,
// so this is a limitation inherited on purpose, not a new one).
var (
	markdownHeadingPattern = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*$`)
	markdownBoldPattern    = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	markdownLinkPattern    = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
)

// ParseMarkdownTopic converts a Markdown document into a HelpTopic: the same
// intermediate representation help_engine.go's .hlf loader produces, so
// HelpView (and anything else that already knows how to show a HelpTopic)
// can display it unchanged. name becomes the topic's Name, the same role a
// .hlf file's "@Name" line plays.
func ParseMarkdownTopic(name, markdown string) *HelpTopic {
	topic := &HelpTopic{Name: name}
	appendLine := func(line string) {
		idx := len(topic.Lines)
		topic.Lines = append(topic.Lines, line)
		parseHelpLinksInto(topic, line, idx)
	}
	for _, raw := range strings.Split(markdown, "\n") {
		if m := markdownHeadingPattern.FindStringSubmatch(raw); m != nil {
			if len(topic.Lines) > 0 && topic.Lines[len(topic.Lines)-1] != "" {
				appendLine("")
			}
			appendLine("#" + stripMarkdownMarkupChars(m[2]) + "#")
			continue
		}
		appendLine(markdownInlineToHelpMarkup(raw))
	}
	return topic
}

// markdownInlineToHelpMarkup rewrites one line's **bold**/__bold__ spans and
// [text](target) links into HelpTopic's own #bold#/~text~target@ markup.
// Bold is resolved first: a bold span nested inside a link's visible text is
// rare enough in practice (and unsupported by the target markup's own
// non-nesting #/~ toggles) that this first part does not attempt it.
func markdownInlineToHelpMarkup(line string) string {
	line = markdownBoldPattern.ReplaceAllStringFunc(line, func(match string) string {
		sub := markdownBoldPattern.FindStringSubmatch(match)
		text := sub[1]
		if text == "" {
			text = sub[2]
		}
		return "#" + stripMarkdownMarkupChars(text) + "#"
	})
	line = markdownLinkPattern.ReplaceAllStringFunc(line, func(match string) string {
		sub := markdownLinkPattern.FindStringSubmatch(match)
		text, target := sub[1], sub[2]
		return "~" + stripMarkdownMarkupChars(text) + "~" + strings.ReplaceAll(target, "@", "%40") + "@"
	})
	return line
}

// stripMarkdownMarkupChars removes the characters HelpTopic's own markup
// gives special meaning to ('#', '~') from text that is about to be wrapped
// in that same markup, so a literal one in the Markdown source cannot be
// mistaken for a bold toggle or a link delimiter it was never meant to be.
func stripMarkdownMarkupChars(text string) string {
	text = strings.ReplaceAll(text, "#", "")
	text = strings.ReplaceAll(text, "~", "")
	return text
}

// NewMarkdownView renders markdown as a scrollable, navigable HelpView --
// the same widget f4's own F1 help uses -- without a preloaded .hlf file or
// a shared *HelpEngine: it builds a private one-topic engine just for this
// document. title becomes both the topic's name and, through HelpView's own
// SwitchTopic, the window's title bar text.
func NewMarkdownView(title, markdown string) *HelpView {
	engine := NewHelpEngine(nil)
	engine.AddTopic(ParseMarkdownTopic(title, markdown))
	return NewHelpView(engine, title)
}
