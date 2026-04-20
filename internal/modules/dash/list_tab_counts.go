package dash

import "swaves/internal/platform/db"

func getRecordTabCounts(dbx *db.DB) (map[string]int, error) {
	categoryCount, err := CountCategoriesService(dbx)
	if err != nil {
		return nil, err
	}
	tagCount, err := CountTagsService(dbx)
	if err != nil {
		return nil, err
	}
	taskCount, err := CountTasksService(dbx)
	if err != nil {
		return nil, err
	}
	redirectCount, err := CountRedirectsService(dbx)
	if err != nil {
		return nil, err
	}
	themeCount, err := db.CountThemes(dbx)
	if err != nil {
		return nil, err
	}

	return map[string]int{
		"categories": categoryCount,
		"tags":       tagCount,
		"tasks":      taskCount,
		"redirects":  redirectCount,
		"themes":     themeCount,
	}, nil
}

func getCommentTabCounts(dbx *db.DB) (map[string]int, error) {
	allCount, err := CountCommentsService(dbx, "")
	if err != nil {
		return nil, err
	}
	pendingCount, err := CountCommentsService(dbx, db.CommentStatusPending)
	if err != nil {
		return nil, err
	}
	approvedCount, err := CountCommentsService(dbx, db.CommentStatusApproved)
	if err != nil {
		return nil, err
	}
	spamCount, err := CountCommentsService(dbx, db.CommentStatusSpam)
	if err != nil {
		return nil, err
	}

	return map[string]int{
		"all":      allCount,
		"pending":  pendingCount,
		"approved": approvedCount,
		"spam":     spamCount,
	}, nil
}
