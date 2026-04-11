package dash

import "testing"

func TestTaskScheduleDisplay(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		want     string
	}{
		{name: "hourly preset", schedule: "@hourly", want: "每小时"},
		{name: "daily preset", schedule: "@daily", want: "每天"},
		{name: "custom cron", schedule: "0 */5 * * *", want: "0 */5 * * *"},
		{name: "blank", schedule: "  ", want: "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskScheduleDisplay(tt.schedule); got != tt.want {
				t.Fatalf("taskScheduleDisplay(%q) = %q, want %q", tt.schedule, got, tt.want)
			}
		})
	}
}
