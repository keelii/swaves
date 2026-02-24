package admin_app

import (
	"strings"
	"swaves/internal/platform/db"
	"time"

	"github.com/gofiber/fiber/v3"
)

type dashboardUVRangeConfig struct {
	Key              string
	Label            string
	BucketSeconds    int64
	BucketCount      int
	PointLabelLayout string
	RangeLabelLayout string
}

type dashboardUVChartResult struct {
	SVG        string
	Total      int
	StartLabel string
	EndLabel   string
}

type dashboardUVTabData struct {
	Key        string
	Label      string
	Active     bool
	SVG        string
	Total      int
	StartLabel string
	EndLabel   string
}

const dashboardActiveUsersWindowSeconds int64 = 30 * 60
const dashboardActiveUsersWindowLabel = "30分钟"

var dashboardUVRanges = []dashboardUVRangeConfig{
	{
		Key:              "24h",
		Label:            "24 小时",
		BucketSeconds:    60 * 60,
		BucketCount:      24,
		PointLabelLayout: "15:00",
		RangeLabelLayout: "01-02 15:04",
	},
	{
		Key:              "7d",
		Label:            "7 天",
		BucketSeconds:    24 * 60 * 60,
		BucketCount:      7,
		PointLabelLayout: "01-02",
		RangeLabelLayout: "01-02",
	},
	{
		Key:              "30d",
		Label:            "30 天",
		BucketSeconds:    24 * 60 * 60,
		BucketCount:      30,
		PointLabelLayout: "01-02",
		RangeLabelLayout: "01-02",
	},
}

func RenderAdminView(c fiber.Ctx, view string, data fiber.Map, layout string) error {
	_ = layout

	if !strings.Contains(view, "/") {
		view = "admin/" + view
	}
	if data == nil {
		data = fiber.Map{}
	}

	routeName := ""
	if route := c.Route(); route != nil {
		routeName = strings.TrimSpace(route.Name)
	}
	data["RouteName"] = routeName
	data["Query"] = c.Queries()
	data["IsLogin"] = fiber.Locals[bool](c, "IsLogin")
	data["_csrf_token_value"] = fiber.Locals[string](c, "CsrfToken")

	return c.Render(view, data)
}

func resolveDashboardUVRange(raw string) dashboardUVRangeConfig {
	raw = strings.TrimSpace(raw)
	for _, item := range dashboardUVRanges {
		if item.Key == raw {
			return item
		}
	}
	return dashboardUVRanges[len(dashboardUVRanges)-1]
}

func localDayStart(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func localHourStart(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, t.Hour(), 0, 0, 0, t.Location())
}

func buildDashboardUVChart(model *db.DB, config dashboardUVRangeConfig, now time.Time) (dashboardUVChartResult, error) {
	var startTime time.Time
	var endTime time.Time
	switch config.Key {
	case "24h":
		endTime = localHourStart(now).Add(time.Hour)
		startTime = endTime.Add(-time.Duration(config.BucketCount) * time.Duration(config.BucketSeconds) * time.Second)
	default:
		endTime = localDayStart(now).Add(24 * time.Hour)
		startTime = endTime.Add(-time.Duration(config.BucketCount) * time.Duration(config.BucketSeconds) * time.Second)
	}

	startAt := startTime.Unix()
	endAt := endTime.Unix()

	buckets, err := db.ListDistinctVisitorsByBucket(model, startAt, endAt, config.BucketSeconds)
	if err != nil {
		return dashboardUVChartResult{}, err
	}

	uvByBucketIndex := make(map[int]int, len(buckets))
	for _, bucket := range buckets {
		if bucket.BucketIndex < 0 || bucket.BucketIndex >= config.BucketCount {
			continue
		}
		uvByBucketIndex[bucket.BucketIndex] = bucket.UV
	}

	points := make([]UVChartPoint, 0, config.BucketCount)
	for i := 0; i < config.BucketCount; i++ {
		bucketStart := startTime.Add(time.Duration(i) * time.Duration(config.BucketSeconds) * time.Second)
		points = append(points, UVChartPoint{
			Label:     bucketStart.Format(config.PointLabelLayout),
			UV:        uvByBucketIndex[i],
			Timestamp: bucketStart.Unix(),
		})
	}

	svg, err := BuildUVChartSVG(UVChartUIData{
		Points:              points,
		ClassName:           "dashboard-uv-svg",
		GridStrokeWidth:     0.4,
		LineStrokeWidth:     0.6,
		Width:               240,
		Height:              32,
		PreserveAspectRatio: "xMinYMid meet",
	})
	if err != nil {
		return dashboardUVChartResult{}, err
	}

	total, err := db.CountDistinctVisitorsBetween(model, startAt, endAt)
	if err != nil {
		return dashboardUVChartResult{}, err
	}

	return dashboardUVChartResult{
		SVG:        svg,
		Total:      total,
		StartLabel: startTime.Format(config.RangeLabelLayout),
		EndLabel:   endTime.Add(-time.Second).Format(config.RangeLabelLayout),
	}, nil
}

func (h *Handler) TestRouter(c fiber.Ctx) error {
	return RenderAdminView(c, "test_home", fiber.Map{
		"Title":       "Test",
		"GetRouteUrl": c.GetRouteURL,
	}, "")
}

func (h *Handler) GetHome(c fiber.Ctx) error {
	totalUV, err := db.CountUVUnique(h.Model, db.UVEntitySite, 0)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	activeUsers, err := db.CountActiveVisitors(h.Model, dashboardActiveUsersWindowSeconds)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	totalLikes, err := db.CountTotalLikes(h.Model)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	postCount, err := db.CountPostsByKind(h.Model, db.PostKindPost)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	pageCount, err := db.CountPostsByKind(h.Model, db.PostKindPage)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	categoryCount, err := db.CountCategories(h.Model)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	tagCount, err := db.CountTags(h.Model)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	topUVContents, err := db.ListTopUVContents(h.Model, 10)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	topLikedContents, err := db.ListTopLikedContents(h.Model, 10)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	activeRange := resolveDashboardUVRange(c.Query("uv_range")).Key
	chartTabs := make([]dashboardUVTabData, 0, len(dashboardUVRanges))
	nowTime := time.Now()
	for _, rangeConfig := range dashboardUVRanges {
		chartData, chartErr := buildDashboardUVChart(h.Model, rangeConfig, nowTime)
		if chartErr != nil {
			return RenderAdminView(c, "admin_home", fiber.Map{
				"Title": "工作台",
				"Error": chartErr.Error(),
			}, "")
		}
		chartTabs = append(chartTabs, dashboardUVTabData{
			Key:        rangeConfig.Key,
			Label:      rangeConfig.Label,
			Active:     rangeConfig.Key == activeRange,
			SVG:        chartData.SVG,
			Total:      chartData.Total,
			StartLabel: chartData.StartLabel,
			EndLabel:   chartData.EndLabel,
		})
	}

	return RenderAdminView(c, "admin_home", fiber.Map{
		"Title":                  "工作台",
		"ActiveUsers":            activeUsers,
		"ActiveUsersWindowLabel": dashboardActiveUsersWindowLabel,
		"TotalUV":                totalUV,
		"TotalLikes":             totalLikes,
		"PostCount":              postCount + pageCount,
		"CategoryCount":          categoryCount,
		"TagCount":               tagCount,
		"TopUVContents":          topUVContents,
		"TopLikedContents":       topLikedContents,
		"UVChartTabs":            chartTabs,
	}, "")
}
