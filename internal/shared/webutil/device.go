package webutil

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

var mobileUserAgentMarkers = []string{
	"mobi",
	"iphone",
	"ipod",
	"ipad",
	"android",
	"windows phone",
	"blackberry",
	"bb10",
	"opera mini",
	"opera mobi",
	"webos",
}

func IsMobileUserAgent(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return false
	}

	for _, marker := range mobileUserAgentMarkers {
		if strings.Contains(ua, marker) {
			return true
		}
	}
	return false
}

func IsMobileRequest(c fiber.Ctx) bool {
	return IsMobileUserAgent(c.UserAgent())
}
