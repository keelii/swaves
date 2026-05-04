package perftrace

import (
	"fmt"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/logger"
	"time"
)

type Trace struct {
	scope   string
	fields  []string
	started time.Time
	last    time.Time
	steps   []Step
}

type Step struct {
	Name       string
	Delta      time.Duration
	SinceStart time.Duration
}

func Enabled() bool {
	return config.EnablePerfTrace
}

func Start(scope string, fields ...string) *Trace {
	if !Enabled() {
		return nil
	}
	now := time.Now()
	return &Trace{
		scope:   strings.TrimSpace(scope),
		fields:  cleanFields(fields),
		started: now,
		last:    now,
	}
}

func (t *Trace) Step(name string) {
	if t == nil {
		return
	}
	now := time.Now()
	t.steps = append(t.steps, Step{
		Name:       strings.TrimSpace(name),
		Delta:      now.Sub(t.last),
		SinceStart: now.Sub(t.started),
	})
	t.last = now
}

func (t *Trace) Finish(fields ...string) {
	if t == nil {
		return
	}
	total := time.Since(t.started)
	if min := time.Duration(config.PerfTraceMinMS) * time.Millisecond; min > 0 && total < min {
		return
	}

	allFields := append([]string{}, t.fields...)
	allFields = append(allFields, cleanFields(fields)...)
	parts := make([]string, 0, len(t.steps))
	for _, step := range t.steps {
		if step.Name == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s@%s", step.Name, roundDuration(step.Delta), roundDuration(step.SinceStart)))
	}
	if len(parts) == 0 {
		parts = append(parts, "steps=none")
	}

	logger.Info("[perf] scope=%s total=%s %s steps=%s",
		emptyAsDash(t.scope),
		roundDuration(total),
		strings.Join(allFields, " "),
		strings.Join(parts, " "),
	)
}

func Field(name string, value any) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "field"
	}
	return fmt.Sprintf("%s=%v", name, value)
}

func cleanFields(fields []string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func emptyAsDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func roundDuration(d time.Duration) time.Duration {
	if d >= time.Second {
		return d.Round(time.Millisecond)
	}
	if d >= time.Millisecond {
		return d.Round(100 * time.Microsecond)
	}
	return d.Round(time.Microsecond)
}
