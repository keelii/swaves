package middleware

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

const requestTrackerSnapshotLimit = 5

type RequestTracker struct {
	nextID atomic.Int64
	active atomic.Int64
	mu     sync.Mutex
	items  map[int64]ActiveRequest
}

type ActiveRequest struct {
	ID        int64
	ReqID     string
	Method    string
	Path      string
	IP        string
	StartedAt time.Time
}

func NewRequestTracker() *RequestTracker {
	return &RequestTracker{
		items: make(map[int64]ActiveRequest),
	}
}

func (rt *RequestTracker) Middleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if rt == nil {
			return c.Next()
		}

		id := rt.nextID.Add(1)
		reqID := strings.TrimSpace(requestid.FromContext(c))
		if reqID == "" {
			reqID = "-"
		}

		info := ActiveRequest{
			ID:        id,
			ReqID:     reqID,
			Method:    strings.TrimSpace(c.Method()),
			Path:      strings.TrimSpace(c.Path()),
			IP:        strings.TrimSpace(c.IP()),
			StartedAt: time.Now(),
		}
		rt.add(info)
		defer rt.remove(id)

		return c.Next()
	}
}

func (rt *RequestTracker) ActiveCount() int64 {
	if rt == nil {
		return 0
	}
	return rt.active.Load()
}

func (rt *RequestTracker) Snapshot(limit int) []ActiveRequest {
	if rt == nil {
		return nil
	}
	if limit <= 0 {
		limit = requestTrackerSnapshotLimit
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	if len(rt.items) == 0 {
		return nil
	}

	items := make([]ActiveRequest, 0, len(rt.items))
	for _, item := range rt.items {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.Before(items[j].StartedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func FormatActiveRequests(items []ActiveRequest, now time.Time) string {
	if len(items) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(items))
	for _, item := range items {
		elapsed := now.Sub(item.StartedAt).Round(time.Millisecond)
		reqID := strings.TrimSpace(item.ReqID)
		if reqID == "" {
			reqID = "-"
		}
		path := strings.TrimSpace(item.Path)
		if path == "" {
			path = "/"
		}
		method := strings.TrimSpace(item.Method)
		if method == "" {
			method = "-"
		}
		ip := strings.TrimSpace(item.IP)
		if ip == "" {
			ip = "-"
		}
		parts = append(parts, fmt.Sprintf("id=%d req_id=%s %s %s ip=%s age=%s", item.ID, reqID, method, path, ip, elapsed))
	}
	return strings.Join(parts, "; ")
}

func (rt *RequestTracker) add(item ActiveRequest) {
	rt.mu.Lock()
	rt.items[item.ID] = item
	rt.mu.Unlock()
	rt.active.Add(1)
}

func (rt *RequestTracker) remove(id int64) {
	rt.mu.Lock()
	if _, ok := rt.items[id]; ok {
		delete(rt.items, id)
		rt.mu.Unlock()
		rt.active.Add(-1)
		return
	}
	rt.mu.Unlock()
}
