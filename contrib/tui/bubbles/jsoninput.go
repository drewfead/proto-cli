package bubbles

import (
	"encoding/json"

	protocli "github.com/drewfead/proto-cli"
)

// NewJSONInput returns a VimInput pre-configured with a JSON validator.
// The resolved default value follows this precedence (highest to lowest):
//  1. userDefault — explicit string passed by the caller in user code
//  2. field.DefaultValue — default declared in proto annotations
//  3. "{\n}" — built-in JSON object template
//  4. "" — empty editor
func NewJSONInput(userDefault string, field protocli.TUIFieldDescriptor, styles Styles) FormControl {
	dv := "{\n}"
	if field.DefaultValue != "" {
		dv = field.DefaultValue
	}
	if userDefault != "" {
		dv = userDefault
	}
	return NewVimInput(dv, styles, func(s string) error {
		var v interface{}
		return json.Unmarshal([]byte(s), &v)
	}, "valid JSON")
}
