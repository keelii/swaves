package dash

import "testing"

func TestParseRedirectStatusStrict(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "empty uses default", raw: "", want: 301, wantErr: false},
		{name: "valid 301", raw: "301", want: 301, wantErr: false},
		{name: "valid 302", raw: "302", want: 302, wantErr: false},
		{name: "invalid status", raw: "307", wantErr: true},
		{name: "invalid non number", raw: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRedirectStatusStrict(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
		})
	}
}
