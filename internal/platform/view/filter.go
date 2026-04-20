package view

import (
	"fmt"
	"math"
	"strings"
	"swaves/internal/platform/config"
	"time"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

func registerViewFilters(env *minijinja.Environment) {
	env.AddFilter("humanSize", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		return value.FromString(formatHumanSize(val.Raw())), nil
	})
	env.AddFilter("formatTime", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		ts, ok := val.AsInt()
		if !ok || ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(time.Unix(ts, 0).Format(config.BaseTimeFormat)), nil
	})
	env.AddFilter("relativeTime", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		ts, ok := val.AsInt()
		if !ok || ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(relativeTimeString(ts)), nil
	})
	env.AddFilter("articleTime", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		ts, ok := val.AsInt()
		if !ok || ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(time.Unix(ts, 0).Format(config.ArticleTimeFormat)), nil
	})
	env.AddFilter("datetimeReplacer", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		text := toStringValue(val.Raw())
		return value.FromString(strings.ReplaceAll(text, "{{year}}", time.Now().Format("2006"))), nil
	})
	env.AddFilter("compactNumber", func(_ minijinja.FilterState, val value.Value, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		return value.FromString(formatCompactNumber(val.Raw())), nil
	})
}

func formatCompactNumber(raw any) string {
	num, ok := toInt64Value(raw)
	if !ok {
		return "0"
	}

	abs := math.Abs(float64(num))
	sign := ""
	if num < 0 {
		sign = "-"
	}

	switch {
	case abs >= 1_000_000_000:
		return sign + compactNumberWithUnit(abs/1_000_000_000, "b")
	case abs >= 1_000_000:
		return sign + compactNumberWithUnit(abs/1_000_000, "m")
	case abs >= 1_000:
		return sign + compactNumberWithUnit(abs/1_000, "k")
	default:
		return fmt.Sprintf("%d", num)
	}
}

func compactNumberWithUnit(value float64, unit string) string {
	if value >= 10 {
		return fmt.Sprintf("%.0f%s", math.Round(value), unit)
	}
	text := fmt.Sprintf("%.1f", math.Round(value*10)/10)
	text = strings.TrimSuffix(text, ".0")
	return text + unit
}

func toInt64Value(raw any) (int64, bool) {
	switch typed := raw.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > math.MaxInt64 {
			return 0, false
		}
		return int64(typed), true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}
