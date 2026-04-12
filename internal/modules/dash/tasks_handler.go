package dash

import (
	"strconv"
	"swaves/internal/platform/db"
	job "swaves/internal/platform/jobs"
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

// Tasks
func (h *Handler) GetTaskListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	tasks, err := ListTasksService(h.Model, &pager)
	if err != nil {
		return err
	}

	recordTabCounts, err := getRecordTabCounts(h.Model)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/tasks_index.html", fiber.Map{
		"Title":              "Tasks",
		"Tasks":              tasks,
		"Pager":              pager,
		"TaskScheduleLabels": buildTaskScheduleLabels(tasks),
		"RecordTabCounts":    recordTabCounts,
	}, "")
}

func (h *Handler) GetTaskNewHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/tasks_new.html", fiber.Map{
		"Title":               "New Task",
		"TaskScheduleOptions": taskScheduleOptions(),
	}, "")
}

func (h *Handler) PostCreateTaskHandler(c fiber.Ctx) error {
	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"

	kind := db.TaskInternal
	if k := c.FormValue("kind"); k == "1" {
		kind = db.TaskUser
	}

	in := CreateTaskInput{
		Code:        c.FormValue("code"),
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		Schedule:    c.FormValue("schedule"),
		Enabled:     enabled,
		Kind:        kind,
	}

	if err := CreateTaskService(h.Model, in); err != nil {
		return err
	}

	return h.redirectToDashRoute(c, "dash.tasks.list", nil, nil)
}

func (h *Handler) GetTaskEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	task, err := GetTaskForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/tasks_edit.html", fiber.Map{
		"Title":               "Edit Task",
		"Task":                task,
		"TaskScheduleOptions": taskScheduleOptions(),
	}, "")
}

func (h *Handler) PostUpdateTaskHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"
	kind := db.TaskInternal
	if k := c.FormValue("kind"); k == "1" {
		kind = db.TaskUser
	}

	in := UpdateTaskInput{
		Code:        c.FormValue("code"),
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		Schedule:    c.FormValue("schedule"),
		Enabled:     enabled,
		Kind:        kind,
	}

	if err := UpdateTaskService(h.Model, id, in); err != nil {
		return err
	}

	return h.redirectToDashRoute(c, "dash.tasks.list", nil, nil)
}

func (h *Handler) PostDeleteTaskHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteTaskService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.tasks.list", nil, nil)
}

func (h *Handler) PostTriggerTaskHandler(c fiber.Ctx) error {
	taskCode := c.Params("code")
	if taskCode == "" {
		return fiber.ErrBadRequest
	}

	task, err := db.GetTaskByCode(h.Model, taskCode)
	if err != nil {
		return err
	}
	go job.ExecuteTask(h.Model, *task)

	return h.redirectToDashRoute(c, "dash.tasks.list", nil, nil)
}

func (h *Handler) GetTaskRunListHandler(c fiber.Ctx) error {
	taskCode := c.Params("code")
	if taskCode == "" {
		return fiber.ErrBadRequest
	}

	// 获取 task 信息
	task, err := db.GetTaskByCode(h.Model, taskCode)
	if err != nil {
		return err
	}

	// 获取执行记录列表，默认限制 100 条
	runs, err := ListTaskRunsService(h.Model, taskCode, 100)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/task_runs_index.html", fiber.Map{
		"Title": "Task Runs: " + task.Name,
		"Task":  task,
		"Runs":  runs,
	}, "")
}
