package view

import (
	"fmt"
	HTML "html"
	"strings"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

func registerViewGlobals(env *minijinja.Environment) {
	env.AddFunction("_csrf_token", func(state *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		token := strings.TrimSpace(toStringValue(state.Lookup("_csrf_token_value").Raw()))
		if token == "" {
			return value.FromSafeString(""), nil
		}
		return value.FromSafeString(fmt.Sprintf(
			`<input type="hidden" name="%s" value="%s">`,
			"_csrf_token",
			HTML.EscapeString(token),
		)), nil
	})
}
