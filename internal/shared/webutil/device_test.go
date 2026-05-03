package webutil

import "testing"

func TestIsMobileUserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		want      bool
	}{
		{
			name:      "iphone",
			userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			want:      true,
		},
		{
			name:      "android",
			userAgent: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36",
			want:      true,
		},
		{
			name:      "ipad",
			userAgent: "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			want:      true,
		},
		{
			name:      "desktop",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			want:      false,
		},
		{
			name:      "empty",
			userAgent: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsMobileUserAgent(tt.userAgent); got != tt.want {
				t.Fatalf("IsMobileUserAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}
