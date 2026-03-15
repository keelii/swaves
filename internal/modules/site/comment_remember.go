package site

import (
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/platform/middleware"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	commentRememberCookieName = "swv_commenter"
	commentRememberCookieAge  = 3600 * 24 * 365
	commentFormFlashTTL       = 10 * time.Minute
)

type commentFormDefaults struct {
	Author      string
	AuthorEmail string
	AuthorURL   string
	Content     string
	RememberMe  bool
}

type commentFormFlashEntry struct {
	Defaults  commentFormDefaults
	ExpiresAt int64
}

var commentFormFlashCache = struct {
	sync.Mutex
	items map[string]commentFormFlashEntry
}{
	items: map[string]commentFormFlashEntry{},
}

func readCommentFormDefaults(c fiber.Ctx, postID int64) commentFormDefaults {
	defaults := parseCommentFormDefaults(c.Cookies(commentRememberCookieName))
	flashDefaults, ok := takeCommentFormFlash(c, postID)
	if !ok {
		return defaults
	}

	defaults.Author = flashDefaults.Author
	defaults.AuthorEmail = flashDefaults.AuthorEmail
	defaults.AuthorURL = flashDefaults.AuthorURL
	defaults.Content = flashDefaults.Content
	defaults.RememberMe = flashDefaults.RememberMe
	return defaults
}

func parseCommentFormDefaults(raw string) commentFormDefaults {
	defaults := commentFormDefaults{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaults
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return defaults
	}

	defaults.Author = strings.TrimSpace(values.Get("author"))
	defaults.AuthorEmail = strings.TrimSpace(values.Get("author_email"))
	defaults.AuthorURL = strings.TrimSpace(values.Get("author_url"))
	defaults.Content = values.Get("content")

	if len(defaults.Author) > 80 {
		defaults.Author = ""
	}
	if len(defaults.AuthorEmail) > 120 {
		defaults.AuthorEmail = ""
	}
	if len(defaults.AuthorURL) > 300 {
		defaults.AuthorURL = ""
	}
	if len(defaults.Content) > 5000 {
		defaults.Content = ""
	}

	defaults.RememberMe = defaults.Author != "" || defaults.AuthorEmail != "" || defaults.AuthorURL != ""
	return defaults
}

func isCommentRememberMeEnabled(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func saveCommentFormDefaults(c fiber.Ctx, defaults commentFormDefaults) {
	if !defaults.RememberMe {
		c.Cookie(&fiber.Cookie{
			Name:     commentRememberCookieName,
			Value:    "",
			Path:     getSiteFallbackPath(),
			HTTPOnly: true,
			SameSite: fiber.CookieSameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
		return
	}

	values := url.Values{}
	values.Set("author", strings.TrimSpace(defaults.Author))
	values.Set("author_email", strings.TrimSpace(defaults.AuthorEmail))
	values.Set("author_url", strings.TrimSpace(defaults.AuthorURL))

	c.Cookie(&fiber.Cookie{
		Name:     commentRememberCookieName,
		Value:    values.Encode(),
		Path:     getSiteFallbackPath(),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		MaxAge:   commentRememberCookieAge,
		Expires:  time.Now().Add(time.Second * commentRememberCookieAge),
	})
}

func commentFormDefaultsFromRequest(c fiber.Ctx) commentFormDefaults {
	defaults := commentFormDefaults{
		Author:      strings.TrimSpace(c.FormValue("author")),
		AuthorEmail: strings.TrimSpace(c.FormValue("author_email")),
		AuthorURL:   strings.TrimSpace(c.FormValue("author_url")),
		Content:     c.FormValue("content"),
		RememberMe:  isCommentRememberMeEnabled(c.FormValue("remember_me")),
	}
	if len(defaults.Author) > 80 {
		defaults.Author = ""
	}
	if len(defaults.AuthorEmail) > 120 {
		defaults.AuthorEmail = ""
	}
	if len(defaults.AuthorURL) > 300 {
		defaults.AuthorURL = ""
	}
	if len(defaults.Content) > 5000 {
		defaults.Content = ""
	}
	return defaults
}

func saveCommentFormFlash(c fiber.Ctx, postID int64, defaults commentFormDefaults) {
	key := commentFormFlashKey(middleware.GetOrCreateVisitorID(c, ""), postID)
	if key == "" {
		return
	}

	commentFormFlashCache.Lock()
	defer commentFormFlashCache.Unlock()

	now := time.Now().Unix()
	pruneCommentFormFlashEntries(now)
	commentFormFlashCache.items[key] = commentFormFlashEntry{
		Defaults:  defaults,
		ExpiresAt: now + int64(commentFormFlashTTL/time.Second),
	}
}

func takeCommentFormFlash(c fiber.Ctx, postID int64) (commentFormDefaults, bool) {
	key := commentFormFlashKey(middleware.GetOrCreateVisitorID(c, ""), postID)
	if key == "" {
		return commentFormDefaults{}, false
	}

	commentFormFlashCache.Lock()
	defer commentFormFlashCache.Unlock()

	now := time.Now().Unix()
	pruneCommentFormFlashEntries(now)
	entry, ok := commentFormFlashCache.items[key]
	if !ok || entry.ExpiresAt <= now {
		delete(commentFormFlashCache.items, key)
		return commentFormDefaults{}, false
	}

	delete(commentFormFlashCache.items, key)
	return entry.Defaults, true
}

func commentFormFlashKey(visitorID string, postID int64) string {
	visitorID = strings.TrimSpace(visitorID)
	if visitorID == "" || postID <= 0 {
		return ""
	}
	return visitorID + ":" + strconv.FormatInt(postID, 10)
}

func pruneCommentFormFlashEntries(now int64) {
	for key, item := range commentFormFlashCache.items {
		if item.ExpiresAt <= now {
			delete(commentFormFlashCache.items, key)
		}
	}
}
