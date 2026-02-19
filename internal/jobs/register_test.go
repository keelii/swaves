package job

import "testing"

func TestShouldSkipTaskRun(t *testing.T) {
	testCases := []struct {
		name     string
		taskCode string
		status   string
		message  string
		wantSkip bool
	}{
		{
			name:     "skip clear_encrypted_posts no-op message",
			taskCode: "clear_encrypted_posts",
			status:   "success",
			message:  "没有发现需要清理的文章\n",
			wantSkip: true,
		},
		{
			name:     "do not skip when status error",
			taskCode: "clear_encrypted_posts",
			status:   "error",
			message:  "没有发现需要清理的文章",
			wantSkip: false,
		},
		{
			name:     "do not skip when task code different",
			taskCode: "remote_backup_data",
			status:   "success",
			message:  "没有发现需要清理的文章",
			wantSkip: false,
		},
		{
			name:     "do not skip when message different",
			taskCode: "clear_encrypted_posts",
			status:   "success",
			message:  "已软删除 2 条过期加密文章",
			wantSkip: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldSkipTaskRun(tc.taskCode, tc.status, tc.message)
			if got != tc.wantSkip {
				t.Fatalf("shouldSkipTaskRun(%q, %q, %q)=%v, want %v", tc.taskCode, tc.status, tc.message, got, tc.wantSkip)
			}
		})
	}
}
