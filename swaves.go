package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"swaves/internal/admin"
	"swaves/internal/api"
	"swaves/internal/consts"
	"swaves/internal/db"
	"swaves/internal/jobs"
	"swaves/internal/middleware"
	"swaves/internal/site"
	"swaves/internal/store"
	"swaves/internal/types"

	"github.com/gofiber/contrib/v3/monitor"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/google/uuid"
)

type SwavesApp struct {
	App    *fiber.App
	Config *types.AppConfig
	Store  *store.GlobalStore
}

func NewApp(config types.AppConfig) SwavesApp {
	globalStore := store.NewGlobalStore(db.Open(db.Options{
		DSN: config.SqliteFile,
	}), admin.NewSessionStore())

	//defer globalStore.Close()

	go job.InitRegistry(globalStore, config) // 初始化定时任务

	store.InitSettings(globalStore)
	view := NewViewEngine()
	app := fiber.New(fiber.Config{
		AppName: config.AppName,
		//DisableStartupMessage: true,
		Views:     view,
		BodyLimit: 10 * 1024 * 1024, // 10MB
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "Internal Server Error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			} else if err != nil {
				msg = err.Error()
			}

			log.Println("[error]HTTP:", code, msg)
			return c.Status(code).SendString(msg)
		},
	})
	RegisterViewFunc(view, app)

	// statics
	app.Use("/static", static.New("./web/static"))
	// metrics
	app.Get("/metrics", monitor.New(monitor.Config{
		Title: config.AppName + " metrics",
	})).Name("system.metrics")

	//app.Use(limiter.New())
	app.Use(middleware.AdminViewContext(globalStore.Session))
	app.Use(middleware.GlobalSettings(consts.GlobalSettingKey))
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header: "X-Req-Id",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
	app.Use(recover.New())
	app.Use(middleware.HttpErrorLog(globalStore.Model))
	//app.Use(logger.New(logger.Config{
	//	TimeFormat: TimeFormat,
	//	Format:     "${time} ${status} - ${method} ${path} ${queryParams} ${body}\n",
	//}))

	//fmt.Println(md.ParseMarkdown(``))

	admin.RegisterRoutes(app, globalStore)
	site.RegisterRoutes(app, globalStore)
	api.RegisterRoutes(app)

	return SwavesApp{
		App:    app,
		Store:  globalStore,
		Config: &config,
	}
}

func (swv *SwavesApp) Listen() {
	log.Println(swv.Config.AppName + " listening on " + swv.Config.ListenAddr)
	log.Fatal(swv.App.Listen(swv.Config.ListenAddr))
}

func (swv *SwavesApp) Shutdown() {
	swv.Store.Close()
}

func newURLForResolver(app *fiber.App) func(name string, params map[string]string, query map[string]string) (string, error) {
	return func(name string, params map[string]string, query map[string]string) (string, error) {
		route := app.GetRoute(strings.TrimSpace(name))
		if strings.TrimSpace(route.Name) == "" {
			return "", fmt.Errorf("route %q not found", name)
		}

		path := route.Path
		consumedKeys := map[string]struct{}{}
		for _, paramName := range route.Params {
			value := strings.TrimSpace(params[paramName])
			if value == "" {
				return "", fmt.Errorf("route %q missing param %q", name, paramName)
			}
			consumedKeys[paramName] = struct{}{}
			path = strings.ReplaceAll(path, ":"+paramName, url.PathEscape(value))
		}

		queryValues := url.Values{}
		for key, value := range params {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			if _, ok := consumedKeys[k]; ok {
				continue
			}
			queryValues.Set(k, value)
		}
		for key, value := range query {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			queryValues.Set(k, value)
		}
		encodedQuery := queryValues.Encode()
		if encodedQuery != "" {
			path += "?" + encodedQuery
		}
		return path, nil
	}
}
