package admin

import (
	"testing"
	"time"
)

func TestMonitorRingBufferSnapshotOrderAndWrap(t *testing.T) {
	buffer := newMonitorRingBuffer(3)
	buffer.Add(monitorHistoryPoint{TS: 1})
	buffer.Add(monitorHistoryPoint{TS: 2})

	firstSnapshot := buffer.Snapshot()
	if len(firstSnapshot) != 2 {
		t.Fatalf("expected 2 points, got %d", len(firstSnapshot))
	}
	if firstSnapshot[0].TS != 1 || firstSnapshot[1].TS != 2 {
		t.Fatalf("unexpected snapshot order: %+v", firstSnapshot)
	}

	buffer.Add(monitorHistoryPoint{TS: 3})
	buffer.Add(monitorHistoryPoint{TS: 4})

	wrapped := buffer.Snapshot()
	if len(wrapped) != 3 {
		t.Fatalf("expected 3 points after wrap, got %d", len(wrapped))
	}
	if wrapped[0].TS != 2 || wrapped[1].TS != 3 || wrapped[2].TS != 4 {
		t.Fatalf("unexpected wrapped order: %+v", wrapped)
	}
}

func TestMonitorRingBufferFullDayWraparound(t *testing.T) {
	capacity := int(monitorRetentionSeconds)
	buffer := newMonitorRingBuffer(capacity)

	base := int64(100)
	for i := 0; i < capacity+10; i++ {
		buffer.Add(monitorHistoryPoint{TS: base + int64(i)})
	}

	snapshot := buffer.Snapshot()
	if len(snapshot) != capacity {
		t.Fatalf("expected %d points, got %d", capacity, len(snapshot))
	}

	expectedFirst := base + 10
	expectedLast := base + int64(capacity) + 9
	if snapshot[0].TS != expectedFirst {
		t.Fatalf("expected first ts=%d, got %d", expectedFirst, snapshot[0].TS)
	}
	if snapshot[len(snapshot)-1].TS != expectedLast {
		t.Fatalf("expected last ts=%d, got %d", expectedLast, snapshot[len(snapshot)-1].TS)
	}
}

func TestAggregateMonitorHistoryBucketCount(t *testing.T) {
	now := time.Unix(1700004060, 0)

	for _, item := range monitorGranularityConfigs {
		points := aggregateMonitorHistory(nil, item, now)
		if len(points) != item.BucketCount {
			t.Fatalf("granularity=%s expected %d points, got %d", item.Key, item.BucketCount, len(points))
		}

		startAt, endAt := monitorWindowRange(item, now)
		if len(points) > 0 && points[0].TS != startAt {
			t.Fatalf("granularity=%s expected first ts=%d, got %d", item.Key, startAt, points[0].TS)
		}
		if len(points) > 1 {
			step := points[1].TS - points[0].TS
			if step != item.BucketSeconds {
				t.Fatalf("granularity=%s expected step=%d, got %d", item.Key, item.BucketSeconds, step)
			}
		}
		if len(points) > 0 {
			lastEnd := points[len(points)-1].TS + item.BucketSeconds
			if lastEnd != endAt {
				t.Fatalf("granularity=%s expected end=%d, got %d", item.Key, endAt, lastEnd)
			}
		}
	}
}

func TestAggregateMonitorHistoryPartialFill(t *testing.T) {
	now := time.Unix(1700004060, 0)
	granularity := monitorGranularityConfigs[0] // 1m

	_, endAt := monitorWindowRange(granularity, now)
	points := []monitorHistoryPoint{
		{TS: endAt - 90, PID: monitorPIDStats{CPU: 10}},
		{TS: endAt - 30, PID: monitorPIDStats{CPU: 20}},
		{TS: endAt - 10, PID: monitorPIDStats{CPU: 40}},
	}

	aggregated := aggregateMonitorHistory(points, granularity, now)
	if len(aggregated) != granularity.BucketCount {
		t.Fatalf("expected %d points, got %d", granularity.BucketCount, len(aggregated))
	}

	last := aggregated[len(aggregated)-1]
	if last.PID.CPU != 30 {
		t.Fatalf("expected last bucket cpu avg=30, got %v", last.PID.CPU)
	}

	prev := aggregated[len(aggregated)-2]
	if prev.PID.CPU != 10 {
		t.Fatalf("expected previous bucket cpu avg=10, got %v", prev.PID.CPU)
	}
}
