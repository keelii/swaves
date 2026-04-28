package dash

import "testing"

func TestMonitorGranularityViewOptions(t *testing.T) {
	rawOptions := monitorGranularityOptions()
	viewOptions := monitorGranularityViewOptions()
	if len(viewOptions) != len(rawOptions) {
		t.Fatalf("view option count = %d, want %d", len(viewOptions), len(rawOptions))
	}

	for idx, raw := range rawOptions {
		item := viewOptions[idx]
		key, ok := item["Key"].(string)
		if !ok {
			t.Fatalf("view option %d key type = %T, want string", idx, item["Key"])
		}
		label, ok := item["Label"].(string)
		if !ok {
			t.Fatalf("view option %d label type = %T, want string", idx, item["Label"])
		}
		if key != raw.Key {
			t.Fatalf("view option %d key = %q, want %q", idx, key, raw.Key)
		}
		if label != raw.Label {
			t.Fatalf("view option %d label = %q, want %q", idx, label, raw.Label)
		}
	}
}

func TestMonitorChartMetricsExcludeApplicationMetrics(t *testing.T) {
	expected := []string{"os_cpu", "os_ram"}
	actual := make([]string, 0, len(monitorChartMetricConfigs))
	for _, metric := range monitorChartMetricConfigs {
		actual = append(actual, metric.Key)
	}

	if len(actual) != len(expected) {
		t.Fatalf("chart metric count = %d, want %d (%v)", len(actual), len(expected), actual)
	}
	for idx, key := range expected {
		if actual[idx] != key {
			t.Fatalf("chart metric %d = %q, want %q", idx, actual[idx], key)
		}
	}
}
