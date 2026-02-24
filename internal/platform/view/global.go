package view

import (
	"errors"
	"fmt"
	HTML "html"
	"strings"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

type csrfTokenInputGlobal struct {
	call func(state value.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error)
}

func (g *csrfTokenInputGlobal) GetAttr(name string) value.Value {
	_ = name
	return value.Undefined()
}

func (g *csrfTokenInputGlobal) ObjectCall(state value.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
	if g == nil || g.call == nil {
		return value.FromSafeString(""), nil
	}
	return g.call(state, args, kwargs)
}

func registerViewGlobals(env *minijinja.Environment) {
	env.AddGlobal("_csrf_token", value.FromObject(&csrfTokenInputGlobal{
		call: func(state value.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
			if len(args) > 0 || len(kwargs) > 0 {
				return value.Undefined(), errors.New("_csrf_token does not support arguments")
			}

			token := strings.TrimSpace(toStringValue(state.Lookup("_csrf_token_value").Raw()))
			if token == "" {
				return value.FromSafeString(""), nil
			}

			return value.FromSafeString(fmt.Sprintf(
				`<input type="hidden" name="%s" value="%s">`,
				"_csrf_token",
				HTML.EscapeString(token),
			)), nil
		},
	}))
}
