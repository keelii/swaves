package config

import "testing"

func TestDashPathRegexp(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "", want: true},
		{input: "/", want: true},
		{input: "dash", want: true},
		{input: "/dash", want: true},
		{input: "dash/dashboard", want: true},
		{input: "/dash/dashboard", want: true},
		{input: "Dash", want: false},
		{input: "/dash-1", want: false},
		{input: "/dash/", want: false},
		{input: "//dash", want: false},
	}

	for _, tt := range tests {
		if got := DashPathRegexp.MatchString(tt.input); got != tt.want {
			t.Fatalf("DashPathRegexp.MatchString(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
