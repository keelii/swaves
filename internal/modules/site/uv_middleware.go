package site

import (
	"strings"
	"swaves/internal/platform/db"

	"github.com/gofiber/fiber/v3"
)

const (
	siteUVEntityTypeLocalKey = "site.uv.entity_type"
	siteUVEntityIDLocalKey   = "site.uv.entity_id"
)

var siteDefaultUVRouteTargets = map[string]db.UVEntityType{
	"site.home":       db.UVEntitySite,
	"site.categories": db.UVEntitySite,
	"site.tags":       db.UVEntitySite,
}

func (h Handler) trackSiteUVMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}

		if !shouldTrackSiteUVRequest(c) {
			return nil
		}

		entityType, entityID, ok := resolveSiteUVTarget(c)
		if !ok {
			return nil
		}

		h.trackEntityUV(c, entityType, entityID)
		return nil
	}
}

func shouldTrackSiteUVRequest(c fiber.Ctx) bool {
	if method := strings.ToUpper(strings.TrimSpace(c.Method())); method != fiber.MethodGet && method != fiber.MethodHead {
		return false
	}

	status := c.Response().StatusCode()
	return status >= fiber.StatusOK && status < fiber.StatusMultipleChoices
}

func resolveSiteUVTarget(c fiber.Ctx) (db.UVEntityType, int64, bool) {
	entityType := fiber.Locals[db.UVEntityType](c, siteUVEntityTypeLocalKey)
	entityID := fiber.Locals[int64](c, siteUVEntityIDLocalKey)
	if entityType.IsValid() && (entityType == db.UVEntitySite || entityID > 0) {
		return entityType, entityID, true
	}

	entityType, ok := siteDefaultUVRouteTargets[currentRouteName(c)]
	if !ok {
		return 0, 0, false
	}

	return entityType, 0, true
}

func currentRouteName(c fiber.Ctx) string {
	if route := c.Route(); route != nil {
		return strings.TrimSpace(route.Name)
	}
	return ""
}

func declareTrackUVEntity(c fiber.Ctx, entityType db.UVEntityType, entityID int64) {
	if !entityType.IsValid() || entityType == db.UVEntitySite || entityID <= 0 {
		return
	}

	c.Locals(siteUVEntityTypeLocalKey, entityType)
	c.Locals(siteUVEntityIDLocalKey, entityID)
}
