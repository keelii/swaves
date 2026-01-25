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

type JobFunc func(reg *Registry) error

var (
	registry *Registry
	mu       sync.Mutex
)

type Registry struct {
	jobs    map[string]JobFunc
	running map[string]bool
	DB      *db.DB
	Config  types.AppConfig
}

// 初始化 Registry
func InitRegistry(gStore *store.GlobalStore, config types.AppConfig) {
	registry = &Registry{
		jobs:    make(map[string]JobFunc),
		running: make(map[string]bool),
		DB:      gStore.Model,
		Config:  config,
	}

	// Internal jobs
	RegisterJob("database_backup", DatabaseBackupJob) // 注册数据库备份任务

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
	var tid int64
	var err error
	tid, err = db.CreateTaskRun(dbx, &db.TaskRun{
		TaskCode:  t.Code,
		Status:    "pending",
		CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		log.Printf("[task] create task run failed: %v", err)
		return
	}

	err = registry.jobs[t.Code](registry)

	status := "success"
	if err != nil {
		status = "error"
	}

	err = db.UpdateTaskRunStatus(dbx, &db.TaskRun{
		ID:         tid,
		Status:     status,
		FinishedAt: time.Now().Unix(),
	})
	if err != nil {
		log.Printf("[task] update task run failed: %v", err)
	}
}

// 注册 Job
func RegisterJob(code string, fn JobFunc) {
	if registry == nil {
		panic("registry not initialized, call InitRegistry first")
	}
	mu.Lock()
	defer mu.Unlock()
	registry.jobs[code] = fn
}

// executor 循环扫描 pending 的 job runs 并执行
func (r *Registry) executor(reg *Registry, interval time.Duration) {
	for {
		time.Sleep(interval)
		r.runPendingJobs(reg)
	}
}

func (r *Registry) runPendingJobs(reg *Registry) {
	runs, err := db.ListTaskRuns(r.DB, "", "pending", 1000) // 扫描所有 pending
	if err != nil {
		log.Printf("[task] fetch pending runs failed: %v", err)
		return
	}

	for _, run := range runs {
		log.Println("[task] fetch pending run:", run)
		// 找 task 函数
		taskCode := run.TaskCode
		fn, ok := r.jobs[taskCode]
		if !ok {
			log.Printf("[task] task %s not registered", taskCode)
			continue
		}

		log.Println("[task] get task:", taskCode)

		mu.Lock()
		if r.running[taskCode] {
			mu.Unlock()
			continue
		}
		r.running[taskCode] = true
		mu.Unlock()

		go func(run db.TaskRun, fn JobFunc) {
			start := time.Now()
			var msg string
			var status string
			log.Println("[task] executing task:", taskCode)
			err := fn(reg)
			if err != nil {
				status = "error"
				msg = err.Error()
				log.Println("[task] task error:", taskCode, err)
			} else {
				status = "success"
				msg = "ok"
				log.Println("[task] task success:", taskCode)
			}
			duration := time.Since(start).Milliseconds()
			run.Status = status
			run.Message = msg
			run.Duration = int64(int(duration))
			run.FinishedAt = time.Now().Unix()

			_ = db.UpdateTaskRunStatus(r.DB, &run)
			db.UpdateTaskStatus(r.DB, taskCode, run.Status, run.FinishedAt)

			mu.Lock()
			r.running[taskCode] = false
			mu.Unlock()
		}(run, fn)
	}
}
