package dash

import (
	"swaves/internal/platform/logger"
	"time"

	"github.com/gofiber/fiber/v3"
)

func (h *Handler) GetMonitorHandler(c fiber.Ctx) error {
	granularity, err := resolveMonitorGranularity(c.Query("granularity", ""))
	if err != nil {
		logger.Warn("[monitor] invalid granularity on page: raw=%s err=%v", c.Query("granularity", ""), err)
		granularity = monitorGranularityConfigs[0]
	}

	return RenderDashView(c, "dash/monitor.html", fiber.Map{
		"Title":             "系统监控",
		"Granularities":     monitorGranularityViewOptions(),
		"ActiveGranularity": granularity.Key,
	}, "")
}

func (h *Handler) GetMetricsAPIHandler(c fiber.Ctx) error {
	point, ok, err := h.Monitor.LatestPoint()
	if err != nil {
		logger.Error("[monitor] metrics api failed: err=%v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "读取监控数据失败",
			"ok":    false,
		})
	}
	if !ok {
		logger.Warn("[monitor] metrics api unavailable: no sample yet")
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
	granularityRaw := c.Query("granularity", "")
	granularity, err := resolveMonitorGranularity(granularityRaw)
	if err != nil {
		logger.Warn("[monitor] monitor api invalid granularity: raw=%s err=%v", granularityRaw, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
			"ok":    false,
		})
	}
	aggregated, latest, err := h.Monitor.Aggregated(time.Now(), granularity)
	if err != nil {
		logger.Error("[monitor] aggregate failed: granularity=%s err=%v", granularity.Key, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "聚合监控数据失败",
			"ok":    false,
		})
	}

	charts := make([]fiber.Map, 0, len(monitorMetricConfigs))
	for _, metric := range monitorMetricConfigs {
		if !metric.ShowChart {
			continue
		}
		chartSVG, buildErr := buildMonitorMetricChartSVG(aggregated, metric, granularity)
		if buildErr != nil {
			logger.Error("[monitor] build chart failed: granularity=%s metric=%s err=%v", granularity.Key, metric.Key, buildErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "生成图表失败",
				"ok":    false,
			})
		}
		charts = append(charts, fiber.Map{
			"metric": monitorMetricOption{Key: metric.Key, Label: metric.Label, Unit: metric.Unit},
			"svg":    chartSVG,
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
		"start_at":      startAt,
		"end_at":        endAt,
		"point_count":   len(aggregated),
		"latest":        latest,
		"charts":        charts,
		"metrics":       monitorMetricOptions(),
		"granularities": monitorGranularityOptions(),
	})
}

func monitorGranularityViewOptions() []fiber.Map {
	options := monitorGranularityOptions()
	items := make([]fiber.Map, 0, len(options))
	for _, item := range options {
		items = append(items, fiber.Map{
			"Key":   item.Key,
			"Label": item.Label,
		})
	}
	return items
}
