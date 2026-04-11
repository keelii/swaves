package dash

import (
	"strings"
	"swaves/internal/platform/db"
)

type taskScheduleOption struct {
	Value string
	Label string
}

var taskSchedulePresetOptions = []taskScheduleOption{
	{Value: "@hourly", Label: "每小时"},
	{Value: "@daily", Label: "每天"},
	{Value: "@weekly", Label: "每周"},
	{Value: "@monthly", Label: "每月"},
	{Value: "@yearly", Label: "每年"},
}

func taskScheduleOptions() []taskScheduleOption {
	options := make([]taskScheduleOption, len(taskSchedulePresetOptions))
	copy(options, taskSchedulePresetOptions)
	return options
}

func taskScheduleDisplay(schedule string) string {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return "-"
	}

	for _, option := range taskSchedulePresetOptions {
		if option.Value == schedule {
			return option.Label
		}
	}
	return schedule
}

func buildTaskScheduleLabels(tasks []db.Task) map[int64]string {
	labels := make(map[int64]string, len(tasks))
	for _, task := range tasks {
		labels[task.ID] = taskScheduleDisplay(task.Schedule)
	}
	return labels
}
