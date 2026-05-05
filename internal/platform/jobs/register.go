package job

import (
	"context"
	"fmt"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/notify"
	"swaves/internal/platform/store"
	"swaves/internal/shared/types"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

// JobFunc 返回值约定：
// - (message, nil): 任务成功，更新任务状态并记录 TaskRun
// - (nil, err): 任务失败，更新任务状态并记录 TaskRun
// - (nil, nil): 任务无动作（no-op），不更新任务最后状态和最后执行时间，不记录 TaskRun
type JobFunc func(reg *Registry) (*string, error)

var (
	registry            *Registry
	registryMu          sync.RWMutex
	mu                  sync.Mutex
	registryInitStarted atomic.Bool
)

type Registry struct {
	jobs    map[string]JobItem
	running map[string]bool
	DB      *db.DB
	Config  types.AppConfig
	cron    *cron.Cron

	cronStopMu      sync.Mutex
	cronStopCtx     context.Context
	cronStopStarted bool
}

type JobItem struct {
	Func JobFunc
	Kind db.TaskKind
}

// 初始化 Registry
func InitRegistry(gStore *store.GlobalStore, config types.AppConfig) {
	if gStore == nil || gStore.Model == nil {
		logger.Warn("[task] init registry skipped: global store/model is nil")
		return
	}
	if !registryInitStarted.CompareAndSwap(false, true) {
		logger.Info("[task] init registry skipped: already initialized")
		return
	}

	reg := &Registry{
		jobs:    make(map[string]JobItem),
		running: make(map[string]bool),
		DB:      gStore.Model,
		Config:  config,
	}
	registryMu.Lock()
	registry = reg
	registryMu.Unlock()

	// Internal jobs
	registerJob("database_backup", JobItem{
		Kind: db.TaskInternal,
		Func: DatabaseBackupJob,
	}) // 注册数据库备份任务

	// 过期加密文章自动删除（用户可在任务管理中创建，建议 schedule 如 @daily）
	registerJob("clear_encrypted_posts", JobItem{
		Kind: db.TaskInternal,
		Func: DeleteExpiredEncryptedPostsJob,
	})

	registerJob("clear_notifications", JobItem{
		Kind: db.TaskInternal,
		Func: ClearExpiredNotificationsJob,
	})

	registerJob("check_app_update", JobItem{
		Kind: db.TaskInternal,
		Func: CheckAppUpdateJob,
	})

	registerJob("remote_backup_data", JobItem{
		Kind: db.TaskUser,
		Func: PushSystemDataJob,
	})

	ensureBuiltinTask(gStore.Model, db.Task{
		Code:        "clear_notifications",
		Name:        "清理过期通知",
		Description: "按消息通知设置中的保留天数清理过期通知",
		Schedule:    "@daily",
		Enabled:     1,
		Kind:        db.TaskInternal,
	})

	ensureBuiltinTask(gStore.Model, db.Task{
		Code:        "check_app_update",
		Name:        "检查应用更新",
		Description: "每天检查 swaves 是否有新的稳定版本可升级",
		Schedule:    "@daily",
		Enabled:     1,
		Kind:        db.TaskInternal,
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
		logger.Error("[task] fetch tasks failed: %v", err)
	}
	c := cron.New()
	reg.cron = c
	for task := range tasks {
		t := tasks[task]

		if t.Enabled != 1 {
			continue
		}
		if _, ok := reg.jobs[t.Code]; !ok {
			logger.Warn("[task] skip unregistered task code: %s", t.Code)
			continue
		}

		logger.Info("add cron job<%s>: %s", t.Schedule, t.Code)
		_, err = c.AddFunc(t.Schedule, func() {
			ExecuteTask(gStore.Model, t)
		})
		if err != nil {
			logger.Error("[task] add cron failed code=%s schedule=%s: %v", t.Code, t.Schedule, err)
		}
	}
	c.Start()
	logger.Info("[task] registry initialized")
}

func ExecuteTask(dbx *db.DB, t db.Task) {
	registryMu.RLock()
	reg := registry
	registryMu.RUnlock()
	if reg == nil {
		logger.Warn("[task] registry not initialized for task: %s", t.Code)
		return
	}
	startAt := time.Now()
	jobItem, ok := reg.jobs[t.Code]
	if !ok {
		err := fmt.Errorf("job not registered: %s", t.Code)
		logger.Error("[task] %v", err)
		if updateErr := db.UpdateTaskStatus(dbx, t.Code, "error", startAt.Unix()); updateErr != nil {
			logger.Error("[task] update task status failed code=%s status=error: %v", t.Code, updateErr)
		}
		notifyTaskResult(dbx, t, "error", err.Error())
		if t.Kind == db.TaskUser {
			if _, createErr := db.CreateTaskRun(dbx, &db.TaskRun{
				TaskCode:   t.Code,
				Status:     "error",
				Message:    err.Error(),
				StartedAt:  startAt.Unix(),
				FinishedAt: time.Now().Unix(),
				Duration:   int64(time.Since(startAt).Milliseconds()),
			}); createErr != nil {
				logger.Error("[task] create task run failed code=%s status=error: %v", t.Code, createErr)
			}
		}
		return
	}

	retPtr, err := jobItem.Func(reg)
	if err == nil && retPtr == nil {
		return
	}

	ret := ""
	if retPtr != nil {
		ret = *retPtr
	}

	status := "success"
	if err != nil {
		status = "error"
		logger.Error("[task] execute job %s failed: %v", t.Code, err)
	} else {
		logger.Info("[task] execute job %s success: %s", t.Code, ret)
	}
	if updateErr := db.UpdateTaskStatus(dbx, t.Code, status, startAt.Unix()); updateErr != nil {
		logger.Error("[task] update task status failed code=%s status=%s: %v", t.Code, status, updateErr)
	}

	message := ret
	if err != nil {
		message = err.Error()
	}
	notifyTaskResult(dbx, t, status, message)

	finishAt := time.Now()
	taskRun := &db.TaskRun{
		TaskCode:   t.Code,
		Status:     status,
		Message:    message,
		StartedAt:  startAt.Unix(),
		FinishedAt: finishAt.Unix(),
		Duration:   int64(finishAt.Sub(startAt).Milliseconds()),
	}

	if _, createErr := db.CreateTaskRun(dbx, taskRun); createErr != nil {
		logger.Error("[task] create task run failed code=%s status=%s: %v", t.Code, status, createErr)
		return
	}
}

func notifyTaskResult(dbx *db.DB, task db.Task, status string, message string) {
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	if normalizedStatus == "success" {
		if !notify.IsTaskSuccessNotificationEnabled() {
			return
		}
	} else if normalizedStatus != "error" {
		return
	}

	if err := notify.CreateTaskResultNotification(dbx, task, normalizedStatus, message, time.Now().Unix()); err != nil {
		logger.Error("[notify] create task notification failed: code=%s status=%s err=%v", task.Code, normalizedStatus, err)
	}
}

// 注册 Job
func registerJob(code string, job JobItem) {
	registryMu.RLock()
	reg := registry
	registryMu.RUnlock()
	if reg == nil {
		panic("registry not initialized, call InitRegistry first")
	}
	mu.Lock()
	defer mu.Unlock()
	reg.jobs[code] = job
}

func DestroyRegistry() {
	startAt := time.Now()
	registryMu.Lock()
	reg := registry
	registry = nil
	registryMu.Unlock()
	if reg == nil {
		registryInitStarted.Store(false)
		return
	}

	waited := reg.waitForCronStop()
	registryInitStarted.Store(false)
	totalElapsed := time.Since(startAt)
	if waited > time.Second || totalElapsed > time.Second {
		logger.Warn("[task] registry destroyed slowly: total_elapsed=%s cron_wait=%s", totalElapsed, waited)
	}
}

func PauseRegistry() {
	registryMu.RLock()
	reg := registry
	registryMu.RUnlock()
	if reg == nil {
		logger.Info("[task] pause registry skipped: registry is nil")
		return
	}

	if reg.requestCronStop() {
		logger.Info("[task] pause registry requested")
		return
	}
	logger.Info("[task] pause registry skipped: cron stop already requested")
}

func (reg *Registry) requestCronStop() bool {
	if reg == nil || reg.cron == nil {
		return false
	}

	reg.cronStopMu.Lock()
	defer reg.cronStopMu.Unlock()
	if reg.cronStopStarted {
		return false
	}
	reg.cronStopCtx = reg.cron.Stop()
	reg.cronStopStarted = true
	return true
}

func (reg *Registry) waitForCronStop() time.Duration {
	if reg == nil || reg.cron == nil {
		return 0
	}

	waitStart := time.Now()
	reg.cronStopMu.Lock()
	stopCtx := reg.cronStopCtx
	if !reg.cronStopStarted {
		stopCtx = reg.cron.Stop()
		reg.cronStopCtx = stopCtx
		reg.cronStopStarted = true
	}
	reg.cronStopMu.Unlock()

	if stopCtx == nil {
		return 0
	}

	<-stopCtx.Done()
	return time.Since(waitStart)
}

func ensureBuiltinTask(dbx *db.DB, task db.Task) {
	_, err := db.GetTaskByCode(dbx, task.Code)
	if err == nil {
		return
	}
	if !db.IsErrNotFound(err) {
		logger.Error("[task] ensure builtin task %s failed: %v", task.Code, err)
		return
	}

	if _, err = db.CreateTask(dbx, &task); err != nil {
		logger.Error("[task] create builtin task %s failed: %v", task.Code, err)
	}
}
