package view

import (
	"errors"
	"strings"
	"swaves/internal/platform/config"
	"time"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

func registerViewFilters(env *minijinja.Environment) {
	env.AddFilter("humanSize", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("humanSize filter does not support keyword arguments")
		}
		return value.FromString(formatHumanSize(val.Raw())), nil
	})
	env.AddFilter("formatTime", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("formatTime filter does not support keyword arguments")
		}
		ts, ok := val.AsInt()
		if !ok || ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(time.Unix(ts, 0).Format(config.BaseTimeFormat)), nil
	})
	env.AddFilter("relativeTime", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("relativeTime filter does not support keyword arguments")
		}
		ts, ok := val.AsInt()
		if !ok || ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(relativeTimeString(ts)), nil
	})
	env.AddFilter("articleTime", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("articleTime filter does not support keyword arguments")
		}
		ts, ok := val.AsInt()
		if !ok || ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(time.Unix(ts, 0).Format(config.ArticleTimeFormat)), nil
	})
	env.AddFilter("datetimeReplacer", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("datetimeReplacer filter does not support keyword arguments")
		}
		text := toStringValue(val.Raw())
		return value.FromString(strings.ReplaceAll(text, "{{year}}", time.Now().Format("2006"))), nil
	})
}
