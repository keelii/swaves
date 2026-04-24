package main

import (
"net/http"
"net/http/httptest"
"sync"
"sync/atomic"
"testing"
"time"
)

func TestCounterRecordsSuccessAndFailure(t *testing.T) {
c := &counter{}
c.record(true, 10*time.Millisecond)
c.record(true, 20*time.Millisecond)
c.record(false, 5*time.Millisecond)

success, failure, totalMS := c.snapshot()
if success != 2 {
t.Errorf("success=%d, want 2", success)
}
if failure != 1 {
t.Errorf("failure=%d, want 1", failure)
}
if totalMS != 35 {
t.Errorf("totalMS=%d, want 35", totalMS)
}
}

func TestSummarizeZero(t *testing.T) {
r := summarize(&counter{}, 5*time.Second)
if r.Total != 0 {
t.Errorf("total=%d, want 0", r.Total)
}
if r.FailRate != 0 {
t.Errorf("fail_rate=%f, want 0", r.FailRate)
}
if r.AvgLatency != 0 {
t.Errorf("avg_latency=%s, want 0", r.AvgLatency)
}
}

func TestSummarizeFailRate(t *testing.T) {
c := &counter{}
c.record(true, 10*time.Millisecond)
c.record(false, 10*time.Millisecond)

r := summarize(c, time.Second)
if r.Total != 2 {
t.Errorf("total=%d, want 2", r.Total)
}
if r.FailRate != 50.0 {
t.Errorf("fail_rate=%.2f, want 50.00", r.FailRate)
}
}

func TestFormatSummaryPass(t *testing.T) {
c := &counter{}
c.record(true, 5*time.Millisecond)

r := summarize(c, time.Second)
s := formatSummary(r)
if len(s) == 0 {
t.Fatal("formatSummary returned empty string")
}
if s[:6] != "[PASS]" {
t.Errorf("expected PASS prefix, got %q", s[:6])
}
}

func TestFormatSummaryFail(t *testing.T) {
c := &counter{}
c.record(false, 5*time.Millisecond)

r := summarize(c, time.Second)
s := formatSummary(r)
if s[:6] != "[FAIL]" {
t.Errorf("expected FAIL prefix, got %q", s[:6])
}
}

func TestFormatProgress(t *testing.T) {
r := result{
Success:    100,
Failure:    2,
Total:      102,
FailRate:   1.96,
AvgLatency: 5 * time.Millisecond,
Elapsed:    10 * time.Second,
}
s := formatProgress(r)
if len(s) == 0 {
t.Fatal("formatProgress returned empty string")
}
}

func TestPidLabel(t *testing.T) {
if pidLabel(0) != "disabled" {
t.Errorf("expected 'disabled' for pid=0")
}
if pidLabel(1234) != "1234" {
t.Errorf("expected '1234' for pid=1234")
}
}

func TestProbeSuccess(t *testing.T) {
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
}))
defer srv.Close()

client := &http.Client{Timeout: 5 * time.Second}
if !probe(client, srv.URL) {
t.Error("expected probe to return true for 200 response")
}
}

func TestProbeServerError(t *testing.T) {
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusInternalServerError)
}))
defer srv.Close()

client := &http.Client{Timeout: 5 * time.Second}
if probe(client, srv.URL) {
t.Error("expected probe to return false for 500 response")
}
}

func TestProbeConnectionRefused(t *testing.T) {
client := &http.Client{Timeout: 100 * time.Millisecond}
// Port 1 is always refused.
if probe(client, "http://127.0.0.1:1/") {
t.Error("expected probe to return false for connection refused")
}
}

func TestRunWorkerStopsOnClosedChannel(t *testing.T) {
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
}))
defer srv.Close()

client := &http.Client{Timeout: 5 * time.Second}
cnt := &counter{}
stop := make(chan struct{})

done := make(chan struct{})
go func() {
defer close(done)
runWorker(srv.URL, client, cnt, stop)
}()

// Let it run briefly, then stop.
time.Sleep(20 * time.Millisecond)
close(stop)

select {
case <-done:
case <-time.After(2 * time.Second):
t.Fatal("runWorker did not stop within 2s after stop channel closed")
}

success, _, _ := cnt.snapshot()
if success == 0 {
t.Error("expected at least one successful request before stopping")
}
}

func TestRunWorkerConcurrency(t *testing.T) {
var inFlight atomic.Int64
var maxInFlight atomic.Int64

srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
cur := inFlight.Add(1)
defer inFlight.Add(-1)
for {
old := maxInFlight.Load()
if cur <= old {
break
}
if maxInFlight.CompareAndSwap(old, cur) {
break
}
}
time.Sleep(2 * time.Millisecond)
w.WriteHeader(http.StatusOK)
}))
defer srv.Close()

client := &http.Client{Timeout: 5 * time.Second}
cnt := &counter{}
stop := make(chan struct{})

const workers = 5
var wg sync.WaitGroup
for i := 0; i < workers; i++ {
wg.Add(1)
go func() {
defer wg.Done()
runWorker(srv.URL, client, cnt, stop)
}()
}

time.Sleep(50 * time.Millisecond)
close(stop)
wg.Wait()

if maxInFlight.Load() < 2 {
t.Errorf("expected concurrent requests >= 2, got max=%d", maxInFlight.Load())
}
}
