package job

import (
	"fmt"
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

	// 过期加密文章自动删除（用户可在任务管理中创建，建议 schedule 如 @daily）
	RegisterJob("clear_encrypted_posts", JobItem{
		Kind: db.TaskInternal,
		Func: DeleteExpiredEncryptedPostsJob,
	})

	RegisterJob("remote_backup_data", JobItem{
		Kind: db.TaskUser,
		Func: PushSystemDataJob,
	})

	ensureBuiltinTask(gStore.Model, db.Task{
		Code:        "remote_backup_data",
		Name:        "系统数据推送",
		Description: "每天推送系统快照到配置的 S3 API 兼容存储（含 R2）",
		Schedule:    "@daily",
		Enabled:     0,
		Kind:        db.TaskUser,
	})

	tasks, err := db.ListTasks(gStore.Model)
	if err != nil {
		log.Printf("[task] fetch tasks failed: %v", err)
	}
	c := cron.New()
	for task := range tasks {
		t := tasks[task]

		if t.Enabled != 1 {
			continue
		}
		if _, ok := registry.jobs[t.Code]; !ok {
			log.Printf("[task] skip unregistered task code: %s", t.Code)
			continue
		}

		log.Printf("add cron job<%s>: %s", t.Schedule, t.Code)
		_, err = c.AddFunc(t.Schedule, func() {
			ExecuteTask(gStore.Model, t)
		})
		if err != nil {
			log.Printf("[task] add cron failed code=%s schedule=%s: %v", t.Code, t.Schedule, err)
		}
	}
	c.Start()
	defer c.Stop()

	select {}
	//go registry.executor(registry, interval)
}

func ExecuteTask(dbx *db.DB, t db.Task) {
	if registry == nil {
		log.Printf("[task] registry not initialized for task: %s", t.Code)
		return
	}

	startAt := time.Now()
	jobItem, ok := registry.jobs[t.Code]
	if !ok {
		err := fmt.Errorf("job not registered: %s", t.Code)
		log.Printf("[task] %v", err)
		_ = db.UpdateTaskStatus(dbx, t.Code, "error", startAt.Unix())
		if t.Kind == db.TaskUser {
			_, _ = db.CreateTaskRun(dbx, &db.TaskRun{
				TaskCode:   t.Code,
				Status:     "error",
				Message:    err.Error(),
				StartedAt:  startAt.Unix(),
				FinishedAt: time.Now().Unix(),
				Duration:   int64(time.Since(startAt).Milliseconds()),
			})
		}
		return
	}

	ret, err := jobItem.Func(registry)
	status := "success"
	if err != nil {
		status = "error"
		log.Printf("[task] execute job %s failed: %v", t.Code, err)
	} else {
		log.Printf("[task] execute job %s success: %s", t.Code, ret)
	}
	_ = db.UpdateTaskStatus(dbx, t.Code, status, startAt.Unix())

	if t.Kind == db.TaskInternal {
		return
	}

	finishAt := time.Now()
	taskRun := &db.TaskRun{
		TaskCode:   t.Code,
		Status:     status,
		Message:    ret,
		StartedAt:  startAt.Unix(),
		FinishedAt: finishAt.Unix(),
		Duration:   int64(finishAt.Sub(startAt).Milliseconds()),
	}
	if err != nil {
		taskRun.Message = err.Error()
	}

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

func ensureBuiltinTask(dbx *db.DB, task db.Task) {
	_, err := db.GetTaskByCode(dbx, task.Code)
	if err == nil {
		return
	}
	if !db.IsErrNotFound(err) {
		log.Printf("[task] ensure builtin task %s failed: %v", task.Code, err)
		return
	}

	if _, err = db.CreateTask(dbx, &task); err != nil {
		log.Printf("[task] create builtin task %s failed: %v", task.Code, err)
	}
}
