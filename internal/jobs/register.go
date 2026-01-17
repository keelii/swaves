package job

import (
	"log"
	"swaves/internal/db"
	"swaves/internal/store"
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
func InitRegistry(gStore *store.GlobalStore, interval time.Duration) {
	registry = &Registry{
		jobs:    make(map[string]JobFunc),
		running: make(map[string]bool),
		DB:      gStore.Model,
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
			err := fn()
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
