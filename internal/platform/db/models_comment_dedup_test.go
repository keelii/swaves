package db

import "testing"

func mustCreateCommentForDedupTest(t *testing.T, dbx *DB, c *Comment) int64 {
	t.Helper()

	id, err := CreateComment(dbx, c)
	if err != nil {
		t.Fatalf("CreateComment failed: %v", err)
	}
	return id
}

func TestCreateCommentRejectsDuplicateByVisitorAndPostContent(t *testing.T) {
	dbx := openTestDB(t)
	post := mustCreatePost(t, dbx, "published", PostKindPost, 0)

	mustCreateCommentForDedupTest(t, dbx, &Comment{
		PostID:    post.ID,
		Author:    "alice",
		VisitorID: "visitor-a",
		AuthorIP:  "127.0.0.1",
		Content:   "same content",
		Status:    CommentStatusPending,
	})

	_, err := CreateComment(dbx, &Comment{
		PostID:    post.ID,
		Author:    "alice",
		VisitorID: "visitor-a",
		AuthorIP:  "127.0.0.1",
		Content:   "same content",
		Status:    CommentStatusPending,
	})
	if err == nil {
		t.Fatal("expected duplicate comment error")
	}
	if !IsErrDuplicateComment(err) {
		t.Fatalf("expected IsErrDuplicateComment=true, got err=%v", err)
	}

	items, listErr := ListPostComments(dbx, post.ID, CommentStatusPending)
	if listErr != nil {
		t.Fatalf("ListPostComments failed: %v", listErr)
	}
	if len(items) != 1 {
		t.Fatalf("duplicate comment should not be inserted, got count=%d", len(items))
	}
}

func TestCreateCommentAllowsSameContentForDifferentVisitor(t *testing.T) {
	dbx := openTestDB(t)
	post := mustCreatePost(t, dbx, "published", PostKindPost, 0)

	mustCreateCommentForDedupTest(t, dbx, &Comment{
		PostID:    post.ID,
		Author:    "alice",
		VisitorID: "visitor-a",
		AuthorURL: "https://example.com/a",
		AuthorIP:  "127.0.0.1",
		Content:   "shared content",
		Status:    CommentStatusPending,
	})

	if _, err := CreateComment(dbx, &Comment{
		PostID:    post.ID,
		Author:    "bob",
		VisitorID: "visitor-b",
		AuthorURL: "https://example.com/b",
		AuthorIP:  "127.0.0.2",
		Content:   "shared content",
		Status:    CommentStatusPending,
	}); err != nil {
		t.Fatalf("different visitor should be allowed, got err=%v", err)
	}
}

func TestCreateCommentRejectsDuplicateByAuthorEmailWhenVisitorChanged(t *testing.T) {
	dbx := openTestDB(t)
	post := mustCreatePost(t, dbx, "published", PostKindPost, 0)

	mustCreateCommentForDedupTest(t, dbx, &Comment{
		PostID:      post.ID,
		Author:      "alice",
		AuthorEmail: "alice@example.com",
		VisitorID:   "visitor-a",
		AuthorIP:    "127.0.0.1",
		Content:     "same content by email",
		Status:      CommentStatusPending,
	})

	_, err := CreateComment(dbx, &Comment{
		PostID:      post.ID,
		Author:      "alice",
		AuthorEmail: "alice@example.com",
		VisitorID:   "visitor-b",
		AuthorIP:    "127.0.0.2",
		Content:     "same content by email",
		Status:      CommentStatusPending,
	})
	if err == nil {
		t.Fatal("expected duplicate comment error when visitor changes but author+email stays the same")
	}
	if !IsErrDuplicateComment(err) {
		t.Fatalf("expected IsErrDuplicateComment=true, got err=%v", err)
	}
}

func TestCreateCommentAllowsSameContentOnDifferentPosts(t *testing.T) {
	dbx := openTestDB(t)
	postA := mustCreatePost(t, dbx, "published", PostKindPost, 0)
	postB := mustCreatePost(t, dbx, "published", PostKindPost, 0)

	mustCreateCommentForDedupTest(t, dbx, &Comment{
		PostID:    postA.ID,
		Author:    "alice",
		VisitorID: "visitor-a",
		AuthorIP:  "127.0.0.1",
		Content:   "same text",
		Status:    CommentStatusPending,
	})

	if _, err := CreateComment(dbx, &Comment{
		PostID:    postB.ID,
		Author:    "alice",
		VisitorID: "visitor-a",
		AuthorIP:  "127.0.0.1",
		Content:   "same text",
		Status:    CommentStatusPending,
	}); err != nil {
		t.Fatalf("same visitor/content on different posts should be allowed: %v", err)
	}
}
