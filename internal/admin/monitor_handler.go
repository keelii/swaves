package admin

import (
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

func (h *Handler) GetMonitorHandler(c fiber.Ctx) error {
	granularity, err := resolveMonitorGranularity(c.Query("granularity", ""))
	if err != nil {
		log.Printf("[monitor] invalid granularity on page: raw=%s err=%v", c.Query("granularity", ""), err)
		granularity = monitorGranularityConfigs[0]
	}

	metric, err := resolveMonitorMetric(c.Query("metric", ""))
	if err != nil {
		log.Printf("[monitor] invalid metric on page: raw=%s err=%v", c.Query("metric", ""), err)
		metric = monitorMetricConfigs[0]
	}

	return RenderAdminView(c, "monitor", fiber.Map{
		"Title":             "系统监控",
		"Granularities":     monitorGranularityOptions(),
		"ActiveGranularity": granularity.Key,
		"Metrics":           monitorMetricOptions(),
		"ActiveMetric":      metric.Key,
	}, "")
}

func (h *Handler) GetMetricsAPIHandler(c fiber.Ctx) error {
	point, ok, err := h.Monitor.LatestPoint()
	if err != nil {
		log.Printf("[monitor] metrics api failed: err=%v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "读取监控数据失败",
			"ok":    false,
		})
	}
	if !ok {
		log.Printf("[monitor] metrics api unavailable: no sample yet")
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "监控数据尚未就绪",
			"ok":    false,
		})
	}

	return c.JSON(fiber.Map{
		"pid": point.PID,
		"os":  point.OS,
		"ts":  point.TS,
	})
}

func (h *Handler) GetMonitorDataAPIHandler(c fiber.Ctx) error {
	granularityRaw := strings.TrimSpace(c.Query("granularity", ""))
	metricRaw := strings.TrimSpace(c.Query("metric", ""))

	granularity, err := resolveMonitorGranularity(granularityRaw)
	if err != nil {
		log.Printf("[monitor] monitor api invalid granularity: raw=%s err=%v", granularityRaw, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
			"ok":    false,
		})
	}

	metric, err := resolveMonitorMetric(metricRaw)
	if err != nil {
		log.Printf("[monitor] monitor api invalid metric: raw=%s err=%v", metricRaw, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
			"ok":    false,
		})
	}

	aggregated, latest, err := h.Monitor.Aggregated(time.Now(), granularity)
	if err != nil {
		log.Printf("[monitor] aggregate failed: granularity=%s metric=%s err=%v", granularity.Key, metric.Key, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "聚合监控数据失败",
			"ok":    false,
		})
	}

	chartSVG, err := buildMonitorMetricChartSVG(aggregated, metric, granularity)
	if err != nil {
		log.Printf("[monitor] build chart failed: granularity=%s metric=%s err=%v", granularity.Key, metric.Key, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "生成图表失败",
			"ok":    false,
		})
	}

	startAt := int64(0)
	endAt := int64(0)
	if len(aggregated) > 0 {
		startAt = aggregated[0].TS
		endAt = aggregated[len(aggregated)-1].TS + granularity.BucketSeconds
		if latest.TS > 0 && latest.TS < endAt {
			endAt = latest.TS
		}
	}

	return c.JSON(fiber.Map{
		"ok":            true,
		"granularity":   granularity,
		"metric":        monitorMetricOption{Key: metric.Key, Label: metric.Label, Unit: metric.Unit},
		"start_at":      startAt,
		"end_at":        endAt,
		"point_count":   len(aggregated),
		"latest":        latest,
		"points":        aggregated,
		"chart_svg":     chartSVG,
		"metrics":       monitorMetricOptions(),
		"granularities": monitorGranularityOptions(),
	})
}
