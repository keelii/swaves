package job

import (
	"log"
	"swaves/internal/db"
	"swaves/internal/store"
	"swaves/internal/types"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

type JobFunc func(reg *Registry) (string, error)

var (
	registry *Registry
	mu       sync.Mutex
)

type Registry struct {
	jobs    map[string]JobItem
	running map[string]bool
	DB      *db.DB
	Config  types.AppConfig
}

type JobItem struct {
	Func JobFunc
	Kind db.TaskKind
}

// 初始化 Registry
func InitRegistry(gStore *store.GlobalStore, config types.AppConfig) {
	registry = &Registry{
		jobs:    make(map[string]JobItem),
		running: make(map[string]bool),
		DB:      gStore.Model,
		Config:  config,
	}

	// Internal jobs
	RegisterJob("database_backup", JobItem{
		Kind: db.TaskInternal,
		Func: DatabaseBackupJob,
	}) // 注册数据库备份任务

	tasks, err := db.ListTasks(gStore.Model)
	if err != nil {
		log.Printf("[task] fetch tasks failed: %v", err)
	}
	c := cron.New()
	for task := range tasks {
		t := tasks[task]

		log.Println("add cron job:", t.Code, t.Schedule)
		//c.AddFunc("@every 10s", func() {
		//	log.Printf("[task] cron job every 10s")
		//})
		c.AddFunc(t.Schedule, func() {
			ExecuteTask(gStore.Model, t)
		})
	}
	c.Start()
	defer c.Stop()

	select {}
	//go registry.executor(registry, interval)
}

func ExecuteTask(dbx *db.DB, t db.Task) {
	var err error
	var ret string

	ret, err = registry.jobs[t.Code].Func(registry)
	if err != nil {
		log.Printf("[task] execute job %s failed: %v", t.Code, err)
	} else {
		log.Printf("[task] execute job %s success: %s", t.Code, ret)
	}

	if t.Kind == db.TaskInternal {
		return
	}

	taskRun := &db.TaskRun{
		TaskCode:  t.Code,
		Status:    "success",
		Message:   ret,
		CreatedAt: time.Now().Unix(),
	}
	if err != nil {
		taskRun.Status = "error"
		taskRun.Message = err.Error()
	}
	taskRun.FinishedAt = time.Now().Unix()

	_, err = db.CreateTaskRun(dbx, taskRun)
	if err != nil {
		log.Printf("[task] create task run failed: %v", err)
		return
	}
}

// 注册 Job
func RegisterJob(code string, job JobItem) {
	if registry == nil {
		panic("registry not initialized, call InitRegistry first")
	}
	mu.Lock()
	defer mu.Unlock()
	registry.jobs[code] = job
}
