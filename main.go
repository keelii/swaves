package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"strings"
	"swaves/internal/admin"
	"swaves/internal/api"
	job "swaves/internal/jobs"
	"swaves/internal/middleware"
	"swaves/internal/store"
	"time"

	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/template/html/v3"
	"github.com/google/uuid"
)

const TimeFormat = "2006-01-02 15:04:05"

//const TimeFormatMs = "2006-01-02 15:04:05.000"

func main() {
	conn := db.Open(db.Options{
		DSN: "data.sqlite",
	})
	defer conn.Close()

	job.InitRegistry(conn, 5*time.Second) // 每 5 秒扫描 pending
	job.RegisterJob("hello", job.HelloJob)
	job.RegisterJob("fdsa", job.HelloJob1)

	store.InitSettings(conn)

	engine := html.New("./web/templates", ".html")
	engine.AddFunc("settings", func(key string) string {
		return store.GetSetting(key)
	})
	engine.AddFunc("dict", func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, errors.New("invalid dict call: must have even number of arguments")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, errors.New("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	})
	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})
	engine.AddFunc("ellipsis", func(a string) template.HTML {
		return template.HTML("<span title='" + a + "' class='ellipsis'>" + a + "</span>")
	})
	engine.AddFunc("long_text", func(a string, cols int, rows int) template.HTML {
		return template.HTML("<textarea class=\"long-text\" cols=\"" + fmt.Sprint(cols) + "\" rows=\"" + fmt.Sprint(rows) + "\" readonly>" + a + "</textarea>")
	})
	engine.AddFunc("until", func(count int) []int {
		var step []int
		for i := 0; i < count; i++ {
			step = append(step, i)
		}
		return step
	})
	engine.AddFunc("formatTime", func(ts interface{}) string {
		var tsInt64 int64
		switch v := ts.(type) {
		case int64:
			tsInt64 = v
		case *int64:
			if v == nil {
				return "-"
			}
			tsInt64 = *v
		default:
			return "-"
		}
		if tsInt64 == 0 {
			return "-"
		}
		return time.Unix(tsInt64, 0).Format(TimeFormat)
	})
	// formatDateTimeLocal 将 Unix timestamp 转换为 datetime-local 输入格式 (YYYY-MM-DDTHH:mm)
	engine.AddFunc("formatDateTimeLocal", func(ts interface{}) string {
		var tsInt64 int64
		switch v := ts.(type) {
		case int64:
			tsInt64 = v
		case *int64:
			if v == nil {
				return ""
			}
			tsInt64 = *v
		default:
			return ""
		}
		if tsInt64 == 0 {
			return ""
		}
		return time.Unix(tsInt64, 0).Format("2006-01-02T15:04")
	})
	// 辅助函数：将 map[string]interface{} 转换为 HTML 属性字符串
	engine.AddFunc("renderAttrs", func(attrs map[string]interface{}) template.HTMLAttr {
		if attrs == nil || len(attrs) == 0 {
			return ""
		}
		var parts []string
		for k, v := range attrs {
			// 将值转换为字符串，处理布尔值（如果为 false 则跳过）
			var val string
			switch tv := v.(type) {
			case bool:
				if tv {
					parts = append(parts, k)
				}
				continue
			case string:
				val = tv
			case float64:
				// JSON 数字会被解析为 float64
				val = fmt.Sprintf("%v", tv)
			default:
				val = fmt.Sprintf("%v", tv)
			}
			// HTML 转义属性值
			val = strings.ReplaceAll(val, `"`, `&quot;`)
			parts = append(parts, fmt.Sprintf(`%s="%s"`, k, val))
		}
		if len(parts) == 0 {
			return ""
		}
		return template.HTMLAttr(" " + strings.Join(parts, " "))
	})
	engine.Reload(true)
	app := fiber.New(fiber.Config{
		AppName:               "swaves",
		DisableStartupMessage: true,
		Views:                 engine,
	})
	app.Static("/static", "./web/static")
	app.Get("/metrics", monitor.New(monitor.Config{Title: "swaves metrics"}))

	//app.Use(limiter.New())
	app.Use(middleware.GlobalSettings("settings"))
	app.Use(requestid.New())
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header:     "X-Req-Id",
		ContextKey: "reqId",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
	app.Use(middleware.HttpErrorLogMiddleware(conn))
	//app.Use(logger.New(logger.Config{
	//	TimeFormat: TimeFormat,
	//	Format:     "${time} ${status} - ${method} ${path} ${queryParams} ${body}\n",
	//}))

	//fmt.Println(md.ParseMarkdown(``))

	admin.RegisterRoutes(app, conn)
	api.RegisterRoutes(app, conn)

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
