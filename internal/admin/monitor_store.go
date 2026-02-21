package admin

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	monitorRetentionSeconds int64 = 24 * 60 * 60
	monitorSampleInterval         = time.Second
)

var monitorGranularityConfigs = []monitorGranularityConfig{
	{Key: "1m", Label: "1分钟", BucketSeconds: 1, BucketCount: 60},
	{Key: "30m", Label: "30分钟", BucketSeconds: 60, BucketCount: 30},
	{Key: "1h", Label: "1小时", BucketSeconds: 60 * 60, BucketCount: 24},
}

type monitorGranularityConfig struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	BucketSeconds int64  `json:"bucket_seconds"`
	BucketCount   int    `json:"bucket_count"`
}

type monitorPIDStats struct {
	CPU   float64 `json:"cpu"`
	RAM   uint64  `json:"ram"`
	Conns int     `json:"conns"`
}

type monitorOSStats struct {
	CPU      float64 `json:"cpu"`
	RAM      uint64  `json:"ram"`
	TotalRAM uint64  `json:"total_ram"`
	LoadAvg  float64 `json:"load_avg"`
	Conns    int     `json:"conns"`
}

type monitorHistoryPoint struct {
	TS  int64           `json:"ts"`
	PID monitorPIDStats `json:"pid"`
	OS  monitorOSStats  `json:"os"`
}

type monitorMetricConfig struct {
	Key         string
	Label       string
	Unit        string
	chartValue  func(point monitorHistoryPoint) int
	formatValue func(point monitorHistoryPoint) string
}

type monitorMetricOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"`
}

var monitorMetricConfigs = []monitorMetricConfig{
	{
		Key:   "pid_cpu",
		Label: "应用 CPU",
		Unit:  "%",
		chartValue: func(point monitorHistoryPoint) int {
			return int(math.Round(point.PID.CPU * 100))
		},
		formatValue: func(point monitorHistoryPoint) string {
			return fmt.Sprintf("%.2f%%", point.PID.CPU)
		},
	},
	{
		Key:   "pid_ram",
		Label: "应用内存",
		Unit:  "B",
		chartValue: func(point monitorHistoryPoint) int {
			return int(math.Round(float64(point.PID.RAM) / (1024 * 1024)))
		},
		formatValue: func(point monitorHistoryPoint) string {
			return formatMonitorBytes(point.PID.RAM)
		},
	},
	{
		Key:   "pid_conns",
		Label: "应用连接数",
		Unit:  "",
		chartValue: func(point monitorHistoryPoint) int {
			return point.PID.Conns
		},
		formatValue: func(point monitorHistoryPoint) string {
			return fmt.Sprintf("%d", point.PID.Conns)
		},
	},
	{
		Key:   "os_cpu",
		Label: "系统 CPU",
		Unit:  "%",
		chartValue: func(point monitorHistoryPoint) int {
			return int(math.Round(point.OS.CPU * 100))
		},
		formatValue: func(point monitorHistoryPoint) string {
			return fmt.Sprintf("%.2f%%", point.OS.CPU)
		},
	},
	{
		Key:   "os_ram",
		Label: "系统内存",
		Unit:  "B",
		chartValue: func(point monitorHistoryPoint) int {
			return int(math.Round(float64(point.OS.RAM) / (1024 * 1024)))
		},
		formatValue: func(point monitorHistoryPoint) string {
			return formatMonitorBytes(point.OS.RAM)
		},
	},
	{
		Key:   "os_load",
		Label: "系统负载",
		Unit:  "",
		chartValue: func(point monitorHistoryPoint) int {
			return int(math.Round(point.OS.LoadAvg * 1000))
		},
		formatValue: func(point monitorHistoryPoint) string {
			return fmt.Sprintf("%.3f", point.OS.LoadAvg)
		},
	},
	{
		Key:   "os_conns",
		Label: "系统连接数",
		Unit:  "",
		chartValue: func(point monitorHistoryPoint) int {
			return point.OS.Conns
		},
		formatValue: func(point monitorHistoryPoint) string {
			return fmt.Sprintf("%d", point.OS.Conns)
		},
	},
}

type MonitorStore struct {
	mu             sync.RWMutex
	buffer         *monitorRingBuffer
	collector      monitorCollector
	interval       time.Duration
	startOnce      sync.Once
	stopOnce       sync.Once
	stopCh         chan struct{}
	startErr       error
	lastCollectErr string
}

type monitorCollector interface {
	Collect(now time.Time) (monitorHistoryPoint, error)
}

type systemMonitorCollector struct {
	process  *process.Process
	cpuCount float64
}

func NewMonitorStore() *MonitorStore {
	collector, err := newSystemMonitorCollector()
	if err != nil {
		log.Printf("[monitor] init collector failed: %v", err)
	}
	return newMonitorStore(collector, monitorSampleInterval, int(monitorRetentionSeconds))
}

func newMonitorStore(collector monitorCollector, interval time.Duration, capacity int) *MonitorStore {
	if interval <= 0 {
		interval = monitorSampleInterval
	}
	if capacity <= 0 {
		capacity = int(monitorRetentionSeconds)
	}
	return &MonitorStore{
		buffer:    newMonitorRingBuffer(capacity),
		collector: collector,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

func (s *MonitorStore) ensureStarted() error {
	s.startOnce.Do(func() {
		if s.collector == nil {
			s.startErr = errors.New("monitor collector is nil")
			log.Printf("[monitor] start failed: %v", s.startErr)
			return
		}

		if err := s.collectAndStore(time.Now()); err != nil {
			log.Printf("[monitor] initial collect failed: %v", err)
		}

		ticker := time.NewTicker(s.interval)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if err := s.collectAndStore(time.Now()); err != nil {
						s.handleCollectError(err)
						continue
					}
					s.clearCollectError()
				case <-s.stopCh:
					return
				}
			}
		}()
	})
	return s.startErr
}

func (s *MonitorStore) collectAndStore(now time.Time) error {
	point, err := s.collector.Collect(now)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.buffer.Add(point)
	s.mu.Unlock()
	return nil
}

func (s *MonitorStore) handleCollectError(err error) {
	errText := strings.TrimSpace(err.Error())
	if errText == "" {
		errText = "unknown error"
	}

	s.mu.Lock()
	if s.lastCollectErr == errText {
		s.mu.Unlock()
		return
	}
	s.lastCollectErr = errText
	s.mu.Unlock()

	log.Printf("[monitor] collect failed: %s", errText)
}

func (s *MonitorStore) clearCollectError() {
	s.mu.Lock()
	hadError := s.lastCollectErr != ""
	s.lastCollectErr = ""
	s.mu.Unlock()

	if hadError {
		log.Printf("[monitor] collect recovered")
	}
}

func (s *MonitorStore) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *MonitorStore) LatestPoint() (monitorHistoryPoint, bool, error) {
	if err := s.ensureStarted(); err != nil {
		return monitorHistoryPoint{}, false, err
	}

	s.mu.RLock()
	latest, ok := s.buffer.Latest()
	s.mu.RUnlock()
	return latest, ok, nil
}

func (s *MonitorStore) Aggregated(now time.Time, granularity monitorGranularityConfig) ([]monitorHistoryPoint, monitorHistoryPoint, error) {
	if err := s.ensureStarted(); err != nil {
		return nil, monitorHistoryPoint{}, err
	}

	s.mu.RLock()
	points := s.buffer.Snapshot()
	latest, ok := s.buffer.Latest()
	s.mu.RUnlock()
	if !ok {
		latest = monitorHistoryPoint{}
	}

	aggregated := aggregateMonitorHistory(points, granularity, now)
	return aggregated, latest, nil
}

func newSystemMonitorCollector() (*systemMonitorCollector, error) {
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, err
	}

	cpuCount := runtime.NumCPU()
	if cpuCount <= 0 {
		cpuCount = 1
	}

	return &systemMonitorCollector{process: proc, cpuCount: float64(cpuCount)}, nil
}

func (c *systemMonitorCollector) Collect(now time.Time) (monitorHistoryPoint, error) {
	point := monitorHistoryPoint{TS: now.Unix()}
	successCount := 0

	if pidCPU, err := c.process.Percent(0); err == nil {
		point.PID.CPU = pidCPU / c.cpuCount
		successCount++
	}

	if osCPU, err := cpu.Percent(0, false); err == nil && len(osCPU) > 0 {
		point.OS.CPU = osCPU[0]
		successCount++
	}

	if pidRAM, err := c.process.MemoryInfo(); err == nil && pidRAM != nil {
		point.PID.RAM = pidRAM.RSS
		successCount++
	}

	if osRAM, err := mem.VirtualMemory(); err == nil && osRAM != nil {
		point.OS.RAM = osRAM.Used
		point.OS.TotalRAM = osRAM.Total
		successCount++
	}

	if loadAvg, err := load.Avg(); err == nil && loadAvg != nil {
		point.OS.LoadAvg = loadAvg.Load1
		successCount++
	}

	if pidConns, err := gopsnet.ConnectionsPid("tcp", c.process.Pid); err == nil {
		point.PID.Conns = len(pidConns)
		successCount++
	}

	if osConns, err := gopsnet.Connections("tcp"); err == nil {
		point.OS.Conns = len(osConns)
		successCount++
	}

	if successCount == 0 {
		return point, errors.New("all monitor probes failed")
	}
	return point, nil
}

func resolveMonitorGranularity(raw string) (monitorGranularityConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return monitorGranularityConfigs[0], nil
	}

	for _, item := range monitorGranularityConfigs {
		if item.Key == raw {
			return item, nil
		}
	}
	return monitorGranularityConfig{}, fmt.Errorf("invalid granularity: %s", raw)
}

func monitorGranularityOptions() []monitorGranularityConfig {
	items := make([]monitorGranularityConfig, len(monitorGranularityConfigs))
	copy(items, monitorGranularityConfigs)
	return items
}

func resolveMonitorMetric(raw string) (monitorMetricConfig, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return monitorMetricConfigs[0], nil
	}

	for _, item := range monitorMetricConfigs {
		if item.Key == raw {
			return item, nil
		}
	}
	return monitorMetricConfig{}, fmt.Errorf("invalid metric: %s", raw)
}

func monitorMetricOptions() []monitorMetricOption {
	items := make([]monitorMetricOption, 0, len(monitorMetricConfigs))
	for _, metric := range monitorMetricConfigs {
		items = append(items, monitorMetricOption{
			Key:   metric.Key,
			Label: metric.Label,
			Unit:  metric.Unit,
		})
	}
	return items
}

func buildMonitorMetricChartSVG(points []monitorHistoryPoint, metric monitorMetricConfig, granularity monitorGranularityConfig) (string, error) {
	chartPoints := make([]UVChartPoint, 0, len(points))
	labelLayout := monitorChartLabelLayout(granularity)

	for _, point := range points {
		label := time.Unix(point.TS, 0).Format(labelLayout)
		chartPoints = append(chartPoints, UVChartPoint{
			Label:     label,
			UV:        metric.chartValue(point),
			Timestamp: point.TS,
			Tooltip:   label + " · " + metric.formatValue(point),
		})
	}

	return BuildUVChartSVG(UVChartUIData{
		Points:              chartPoints,
		ClassName:           "monitor-chart-svg",
		Width:               920,
		Height:              220,
		GridStrokeWidth:     0.35,
		LineStrokeWidth:     1.1,
		PointRadius:         1.2,
		PreserveAspectRatio: "none",
	})
}

func monitorChartLabelLayout(granularity monitorGranularityConfig) string {
	if granularity.BucketSeconds < 60 {
		return "15:04:05"
	}
	if granularity.BucketSeconds >= 60*60 {
		return "01-02 15:04"
	}
	return "15:04"
}

func aggregateMonitorHistory(points []monitorHistoryPoint, granularity monitorGranularityConfig, now time.Time) []monitorHistoryPoint {
	startAt, endAt := monitorWindowRange(granularity, now)
	result := make([]monitorHistoryPoint, granularity.BucketCount)
	for i := range result {
		result[i].TS = startAt + int64(i)*granularity.BucketSeconds
	}
	if len(points) == 0 {
		return result
	}

	buckets := make([]monitorBucketAccumulator, granularity.BucketCount)
	for _, point := range points {
		if point.TS < startAt || point.TS >= endAt {
			continue
		}

		index := int((point.TS - startAt) / granularity.BucketSeconds)
		if index < 0 || index >= len(buckets) {
			continue
		}
		buckets[index].add(point)
	}

	for i := range result {
		if buckets[i].count == 0 {
			continue
		}
		result[i].PID = buckets[i].avgPID()
		result[i].OS = buckets[i].avgOS()
	}

	return result
}

func monitorWindowRange(granularity monitorGranularityConfig, now time.Time) (int64, int64) {
	endAt := now.Unix()
	if endAt < 0 {
		endAt = 0
	}
	if granularity.BucketSeconds > 0 {
		aligned := endAt - endAt%granularity.BucketSeconds
		if aligned < endAt {
			endAt = aligned + granularity.BucketSeconds
		} else {
			endAt = aligned
		}
	}
	startAt := endAt - int64(granularity.BucketCount)*granularity.BucketSeconds
	return startAt, endAt
}

type monitorBucketAccumulator struct {
	count float64

	pidCPUSum   float64
	pidRAMSum   float64
	pidConnsSum float64

	osCPUSum      float64
	osRAMSum      float64
	osTotalRAMSum float64
	osLoadSum     float64
	osConnsSum    float64
}

func (a *monitorBucketAccumulator) add(point monitorHistoryPoint) {
	a.count++
	a.pidCPUSum += point.PID.CPU
	a.pidRAMSum += float64(point.PID.RAM)
	a.pidConnsSum += float64(point.PID.Conns)
	a.osCPUSum += point.OS.CPU
	a.osRAMSum += float64(point.OS.RAM)
	a.osTotalRAMSum += float64(point.OS.TotalRAM)
	a.osLoadSum += point.OS.LoadAvg
	a.osConnsSum += float64(point.OS.Conns)
}

func (a monitorBucketAccumulator) avgPID() monitorPIDStats {
	return monitorPIDStats{
		CPU:   a.pidCPUSum / a.count,
		RAM:   uint64(math.Round(a.pidRAMSum / a.count)),
		Conns: int(math.Round(a.pidConnsSum / a.count)),
	}
}

func (a monitorBucketAccumulator) avgOS() monitorOSStats {
	return monitorOSStats{
		CPU:      a.osCPUSum / a.count,
		RAM:      uint64(math.Round(a.osRAMSum / a.count)),
		TotalRAM: uint64(math.Round(a.osTotalRAMSum / a.count)),
		LoadAvg:  a.osLoadSum / a.count,
		Conns:    int(math.Round(a.osConnsSum / a.count)),
	}
}

type monitorRingBuffer struct {
	points []monitorHistoryPoint
	next   int
	count  int
}

func newMonitorRingBuffer(capacity int) *monitorRingBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &monitorRingBuffer{points: make([]monitorHistoryPoint, capacity)}
}

func (b *monitorRingBuffer) Add(point monitorHistoryPoint) {
	b.points[b.next] = point
	b.next = (b.next + 1) % len(b.points)
	if b.count < len(b.points) {
		b.count++
	}
}

func (b *monitorRingBuffer) Snapshot() []monitorHistoryPoint {
	if b.count == 0 {
		return nil
	}
	items := make([]monitorHistoryPoint, b.count)
	if b.count < len(b.points) {
		copy(items, b.points[:b.count])
		return items
	}

	for i := 0; i < b.count; i++ {
		idx := (b.next + i) % len(b.points)
		items[i] = b.points[idx]
	}
	return items
}

func (b *monitorRingBuffer) Latest() (monitorHistoryPoint, bool) {
	if b.count == 0 {
		return monitorHistoryPoint{}, false
	}
	index := b.next - 1
	if index < 0 {
		index = len(b.points) - 1
	}
	return b.points[index], true
}

func formatMonitorBytes(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	size := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIdx := 0
	for size >= 1024 && unitIdx < len(units)-1 {
		size /= 1024
		unitIdx++
	}

	text := fmt.Sprintf("%.2f", size)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	return text + " " + units[unitIdx]
}
