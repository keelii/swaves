package job

import (
	"log"
	"swaves/internal/db"
	"sync"
	"time"
)

type JobFunc func() error

var (
	registry *Registry
	mu       sync.Mutex
)

type Registry struct {
	jobs    map[string]JobFunc
	running map[string]bool
	DB      *db.DB
}

// 初始化 Registry
func InitRegistry(dbx *db.DB, interval time.Duration) {
	registry = &Registry{
		jobs:    make(map[string]JobFunc),
		running: make(map[string]bool),
		DB:      dbx,
	}
	go registry.executor(interval)
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
func (r *Registry) executor(interval time.Duration) {
	for {
		time.Sleep(interval)
		r.runPendingJobs()
	}
}

func (r *Registry) runPendingJobs() {
	runs, err := db.ListCronJobRuns(r.DB, "", "pending", 1000) // 扫描所有 pending
	if err != nil {
		log.Printf("[cron] fetch pending runs failed: %v", err)
		return
	}

	for _, run := range runs {
		log.Println("[cron] fetch pending run:", run)
		// 找 job 函数
		jobCode := run.JobCode
		fn, ok := r.jobs[jobCode]
		if !ok {
			log.Printf("[cron] job %s not registered", jobCode)
			continue
		}

		log.Println("[cron] get job:", jobCode)

		mu.Lock()
		if r.running[jobCode] {
			mu.Unlock()
			continue
		}
		r.running[jobCode] = true
		mu.Unlock()

		go func(run db.CronJobRun, fn JobFunc) {
			start := time.Now()
			var msg string
			var status string
			log.Println("[cron] executing job:", jobCode)
			err := fn()
			if err != nil {
				status = "error"
				msg = err.Error()
				log.Println("[cron] job error:", jobCode, err)
			} else {
				status = "success"
				msg = "ok"
				log.Println("[cron] job success:", jobCode)
			}
			duration := time.Since(start).Milliseconds()
			run.Status = status
			run.Message = msg
			run.Duration = int64(int(duration))
			run.FinishedAt = time.Now().Unix()

			_ = db.UpdateCronJobRunStatus(r.DB, &run)
			db.UpdateCronJobStatus(r.DB, jobCode, run.Status, run.FinishedAt)
			//_ = db.CreateCronJobRun(r.DB, &run)

			mu.Lock()
			r.running[jobCode] = false
			mu.Unlock()
		}(run, fn)
	}
}
