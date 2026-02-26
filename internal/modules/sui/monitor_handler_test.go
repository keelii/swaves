package sui

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
