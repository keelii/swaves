package site

import (
	"fmt"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"sync"
	"time"
)

const (
	uvTrackQueueSize      = 1024
	uvTrackDrainTimeout   = 2 * time.Second
	uvTrackPruneThreshold = 4096
)

type UVTracker struct {
	model   *db.DB
	events  chan uvTrackEvent
	done    chan struct{}
	mu      sync.Mutex
	seen    map[string]int64
	closing bool
}

type uvTrackEvent struct {
	EntityType db.UVEntityType
	EntityID   int64
	VisitorID  string
	SeenAt     int64
}

func NewUVTracker(model *db.DB) *UVTracker {
	tracker := &UVTracker{
		model:  model,
		events: make(chan uvTrackEvent, uvTrackQueueSize),
		done:   make(chan struct{}),
		seen:   make(map[string]int64),
	}
	go tracker.run()
	return tracker
}

func (t *UVTracker) Track(entityType db.UVEntityType, entityID int64, visitorID string) {
	if t == nil || t.model == nil || visitorID == "" {
		return
	}
	if !entityType.IsValid() {
		return
	}

	event := uvTrackEvent{
		EntityType: entityType,
		EntityID:   entityID,
		VisitorID:  visitorID,
		SeenAt:     time.Now().Unix(),
	}
	t.enqueue(event)
}

func (t *UVTracker) Close() {
	if t == nil {
		return
	}

	t.mu.Lock()
	if t.closing {
		t.mu.Unlock()
		return
	}
	t.closing = true
	close(t.events)
	t.mu.Unlock()

	select {
	case <-t.done:
	case <-time.After(uvTrackDrainTimeout):
		logger.Warn("[uv] tracker shutdown timed out")
	}
}

func (t *UVTracker) run() {
	defer close(t.done)
	for event := range t.events {
		if _, err := db.UpsertUVUnique(t.model, event.EntityType, event.EntityID, event.VisitorID); err != nil {
			t.forget(event)
			logger.Warn("[uv] track failed: entity_type=%d entity_id=%d err=%v", event.EntityType, event.EntityID, err)
		}
	}
}

func (t *UVTracker) enqueue(event uvTrackEvent) {
	key := event.key()
	threshold := event.SeenAt - db.UVLastSeenUpdateMinIntervalSeconds

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closing {
		return
	}
	if lastSeenAt, ok := t.seen[key]; ok && lastSeenAt > threshold {
		return
	}
	t.seen[key] = event.SeenAt
	if len(t.seen) > uvTrackPruneThreshold {
		t.pruneLocked(threshold)
	}

	select {
	case t.events <- event:
	default:
		delete(t.seen, key)
		logger.Warn("[uv] track queue full: entity_type=%d entity_id=%d", event.EntityType, event.EntityID)
	}
}

func (t *UVTracker) forget(event uvTrackEvent) {
	key := event.key()
	t.mu.Lock()
	if t.seen[key] == event.SeenAt {
		delete(t.seen, key)
	}
	t.mu.Unlock()
}

func (t *UVTracker) pruneLocked(threshold int64) {
	for key, lastSeenAt := range t.seen {
		if lastSeenAt <= threshold {
			delete(t.seen, key)
		}
	}
}

func (e uvTrackEvent) key() string {
	return fmt.Sprintf("%d:%d:%s", e.EntityType, e.EntityID, e.VisitorID)
}
