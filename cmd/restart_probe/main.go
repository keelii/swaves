// restart_probe is a concurrency stress-test tool that validates zero-downtime
// restarts for the swaves daemon.
//
// It hammers a target URL with concurrent HTTP workers throughout the test
// window.  When --pid is provided it also triggers SIGHUP on the master process
// at --restart-interval so that each full restart cycle is exercised
// automatically.
//
// Usage:
//
// go run ./cmd/restart_probe --url http://localhost:4096/ --pid 12345
// go run ./cmd/restart_probe --url http://localhost:4096/ --pid 12345 --concurrency 20 --duration 30s --restart-interval 5s
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	flagURL              = flag.String("url", "http://localhost:4096/", "target URL to probe")
	flagConcurrency      = flag.Int("concurrency", 10, "number of concurrent request workers")
	flagDuration         = flag.Duration("duration", 30*time.Second, "total test duration (0 = run until interrupted)")
	flagPID              = flag.Int("pid", 0, "master process PID to send SIGHUP (0 = disabled)")
	flagRestartInterval  = flag.Duration("restart-interval", 8*time.Second, "interval between SIGHUP signals")
	flagProgressInterval = flag.Duration("progress-interval", time.Second, "interval for printing live progress")
)

type counter struct {
	success atomic.Int64
	failure atomic.Int64
	latency atomic.Int64 // total latency in milliseconds
}

func (c *counter) record(ok bool, elapsed time.Duration) {
	ms := elapsed.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	c.latency.Add(ms)
	if ok {
		c.success.Add(1)
	} else {
		c.failure.Add(1)
	}
}

func (c *counter) snapshot() (success, failure, totalMS int64) {
	return c.success.Load(), c.failure.Load(), c.latency.Load()
}

type result struct {
	Success    int64
	Failure    int64
	Total      int64
	FailRate   float64
	AvgLatency time.Duration
	Elapsed    time.Duration
}

func summarize(c *counter, elapsed time.Duration) result {
	success, failure, totalMS := c.snapshot()
	total := success + failure
	var failRate float64
	var avgLatency time.Duration
	if total > 0 {
		failRate = float64(failure) / float64(total) * 100
		avgLatency = time.Duration(totalMS/total) * time.Millisecond
	}
	return result{
		Success:    success,
		Failure:    failure,
		Total:      total,
		FailRate:   failRate,
		AvgLatency: avgLatency,
		Elapsed:    elapsed.Round(time.Millisecond),
	}
}

func formatProgress(r result) string {
	return fmt.Sprintf(
		"elapsed=%-8s  total=%-8d  ok=%-8d  fail=%-8d  fail%%=%-6.2f  avg_latency=%s",
		r.Elapsed.Round(time.Second),
		r.Total,
		r.Success,
		r.Failure,
		r.FailRate,
		r.AvgLatency,
	)
}

func formatSummary(r result) string {
	verdict := "PASS"
	if r.Failure > 0 {
		verdict = "FAIL"
	}
	return fmt.Sprintf(
		"[%s] total=%d  ok=%d  fail=%d  fail%%=%.2f  avg_latency=%s  elapsed=%s",
		verdict,
		r.Total,
		r.Success,
		r.Failure,
		r.FailRate,
		r.AvgLatency,
		r.Elapsed,
	)
}

func runWorker(url string, client *http.Client, cnt *counter, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		start := time.Now()
		ok := probe(client, url)
		cnt.record(ok, time.Since(start))
	}
}

func probe(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}

func sendSIGHUP(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process pid=%d: %w", pid, err)
	}
	return proc.Signal(syscall.SIGHUP)
}

func pidLabel(pid int) string {
	if pid == 0 {
		return "disabled"
	}
	return strconv.Itoa(pid)
}

func main() {
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [options]\n\n", "restart_probe")
		fmt.Fprintln(out, "Hammer a URL with concurrent requests while optionally triggering daemon")
		fmt.Fprintln(out, "restarts via SIGHUP.  Prints live progress and a final summary.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Examples:")
		fmt.Fprintln(out, "  # Run for 30 s with 10 workers, trigger SIGHUP every 8 s:")
		fmt.Fprintln(out, "  go run ./cmd/restart_probe --url http://localhost:4096/ --pid $(cat /tmp/swaves.pid)")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  # Run until Ctrl+C without triggering restarts:")
		fmt.Fprintln(out, "  go run ./cmd/restart_probe --url http://localhost:4096/ --duration 0")
		fmt.Fprintln(out)
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected extra arguments: %s\n\n", strings.Join(flag.Args(), " "))
		flag.Usage()
		os.Exit(2)
	}

	targetURL := strings.TrimSpace(*flagURL)
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "--url is required")
		os.Exit(2)
	}
	concurrency := *flagConcurrency
	if concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "--concurrency must be > 0")
		os.Exit(2)
	}
	masterPID := *flagPID
	if masterPID < 0 {
		fmt.Fprintln(os.Stderr, "--pid must be >= 0")
		os.Exit(2)
	}

	fmt.Fprintf(os.Stdout, "restart_probe starting\n")
	fmt.Fprintf(os.Stdout, "  url=%s  concurrency=%d  duration=%s  pid=%s  restart-interval=%s\n\n",
		targetURL, concurrency, *flagDuration, pidLabel(masterPID), *flagRestartInterval)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: concurrency,
			DisableKeepAlives:   false,
		},
	}

	cnt := &counter{}
	stopWorkers := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(targetURL, client, cnt, stopWorkers)
		}()
	}

	startAt := time.Now()

	progressTicker := time.NewTicker(*flagProgressInterval)
	defer progressTicker.Stop()

	var restartTicker *time.Ticker
	var restartCh <-chan time.Time
	if masterPID > 0 {
		restartTicker = time.NewTicker(*flagRestartInterval)
		restartCh = restartTicker.C
		defer restartTicker.Stop()
	}

	var durationTimer <-chan time.Time
	if *flagDuration > 0 {
		t := time.NewTimer(*flagDuration)
		durationTimer = t.C
		defer t.Stop()
	}

	intCh := make(chan os.Signal, 1)
	signal.Notify(intCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(intCh)

	restartCount := 0
	done := false
	for !done {
		select {
		case <-progressTicker.C:
			r := summarize(cnt, time.Since(startAt))
			fmt.Printf("\r%s", formatProgress(r))

		case <-restartCh:
			restartCount++
			if err := sendSIGHUP(masterPID); err != nil {
				fmt.Fprintf(os.Stderr, "\n[restart #%d] SIGHUP failed: %v\n", restartCount, err)
			} else {
				fmt.Fprintf(os.Stdout, "\n[restart #%d] SIGHUP sent to pid=%d\n", restartCount, masterPID)
			}

		case <-durationTimer:
			fmt.Fprintln(os.Stdout, "\nduration reached, stopping workers…")
			done = true

		case sig := <-intCh:
			fmt.Fprintf(os.Stdout, "\nreceived %s, stopping workers…\n", sig)
			done = true
		}
	}

	close(stopWorkers)
	wg.Wait()

	elapsed := time.Since(startAt)
	r := summarize(cnt, elapsed)
	fmt.Println()
	fmt.Println("─────────────────────────────────────────────────────────")
	fmt.Println(formatSummary(r))
	fmt.Printf("restarts triggered: %d\n", restartCount)
	fmt.Println("─────────────────────────────────────────────────────────")

	if r.Failure > 0 {
		os.Exit(1)
	}
}
