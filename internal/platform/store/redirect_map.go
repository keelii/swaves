package store

import (
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/pathutil"
	"sync/atomic"
)

type RedirectRule struct {
	To     string
	Status int
}

var Redirects atomic.Value
var redirectEmpty atomic.Bool
var dbChangeStore atomic.Value
var dbChangeHandlerRegistered atomic.Bool

func init() {
	redirectEmpty.Store(false)
}

func storeRedirectMap(m map[string]RedirectRule) {
	if m == nil {
		m = map[string]RedirectRule{}
	}

	Redirects.Store(m)
	redirectEmpty.Store(len(m) == 0)
}

func registerDatabaseChangeHandler(gStore *GlobalStore) {
	if gStore == nil {
		return
	}

	dbChangeStore.Store(gStore)
	if !dbChangeHandlerRegistered.CompareAndSwap(false, true) {
		return
	}

	db.OnDatabaseChanged = func(tableName db.TableName, kind db.TableOp) {
		if kind != db.TableOpInsert && kind != db.TableOpUpdate && kind != db.TableOpDelete {
			return
		}

		activeStore, _ := dbChangeStore.Load().(*GlobalStore)
		if activeStore == nil || activeStore.IsClosed() {
			return
		}

		switch tableName {
		case db.TableSettings:
			if err := ReloadSettings(activeStore); err != nil {
				logger.Error("reload settings failed: %v", err)
			}
		case db.TableRedirects:
			if err := ReloadRedirects(activeStore); err != nil {
				logger.Error("reload redirects failed: %v", err)
			}
		}
	}
}

func InitRedirects(gStore *GlobalStore) {
	if err := ReloadRedirects(gStore); err != nil {
		logger.Fatal("initial redirects load failed: %v", err)
	}
	registerDatabaseChangeHandler(gStore)
}

func ReloadRedirects(gStore *GlobalStore) error {
	if gStore == nil || gStore.IsClosed() {
		return nil
	}

	m, err := db.LoadRedirectsToMap(gStore.Model)
	if err != nil {
		logger.Error("error loading redirects: %v", err)
		return err
	}

	redirectMap := make(map[string]RedirectRule, len(m))
	for from, redirect := range m {
		redirectMap[from] = RedirectRule{
			To:     redirect.To,
			Status: redirect.Status,
		}
	}

	storeRedirectMap(redirectMap)
	logger.Info("redirects loaded successfully: count=%d", len(redirectMap))
	return nil
}

func GetRedirect(path string) (RedirectRule, bool) {
	m, ok := Redirects.Load().(map[string]RedirectRule)
	if !ok {
		storeRedirectMap(map[string]RedirectRule{})
		return RedirectRule{}, false
	}

	redirect, exists := m[path]
	if exists {
		return redirect, true
	}

	normalizedPath := normalizeRedirectLookupPath(path)
	if normalizedPath == "" || normalizedPath == path {
		return RedirectRule{}, false
	}

	redirect, exists = m[normalizedPath]
	return redirect, exists
}

func GetRedirectMap() map[string]RedirectRule {
	m, ok := Redirects.Load().(map[string]RedirectRule)
	if !ok {
		storeRedirectMap(map[string]RedirectRule{})
		return map[string]RedirectRule{}
	}
	return m
}

func IsRedirectEmpty() bool {
	return redirectEmpty.Load()
}

func normalizeRedirectLookupPath(path string) string {
	path = pathutil.JoinAbsolute(path)
	if path == "/" {
		return ""
	}
	return path
}
