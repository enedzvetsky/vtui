package vtui

import "github.com/unxed/vtui/vreactive"

// BindText binds the edit's text to a property in both directions: the text
// follows the property, and typing updates it. The property's current value is
// applied at once. The returned function undoes the binding and restores the
// edit's previous OnTextChange handler. Notification echoes between the two
// sides are suppressed by vreactive.TwoWayBind.
func (e *Edit) BindText(prop vreactive.Property[string]) func() {
	if e == nil || prop == nil {
		return func() {}
	}
	previous := e.OnTextChange
	return vreactive.TwoWayBind(prop, e.GetText, e.SetText, func(onChange func(string)) func() {
		e.OnTextChange = func(text string) {
			if previous != nil {
				previous(text)
			}
			onChange(text)
		}
		return func() { e.OnTextChange = previous }
	})
}
