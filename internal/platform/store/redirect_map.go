package store

import (
	"errors"
	"fmt"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/pathutil"
	"swaves/internal/shared/redirect_rule"
	"sync/atomic"
)

type RedirectRule struct {
	To     string
	Status int
}

type compiledRedirectRule struct {
	rule   redirect_rule.Rule
	status int
}

var Redirects atomic.Value
var RedirectPatterns atomic.Value
var redirectEmpty atomic.Bool
var dbChangeStore atomic.Value
var dbChangeHandlerRegistered atomic.Bool

func init() {
	redirectEmpty.Store(false)
	RedirectPatterns.Store([]compiledRedirectRule{})
}

func storeRedirectMap(m map[string]RedirectRule, patterns []compiledRedirectRule) {
	if m == nil {
		m = map[string]RedirectRule{}
	}
	if patterns == nil {
		patterns = []compiledRedirectRule{}
	}

	Redirects.Store(m)
	RedirectPatterns.Store(patterns)
	redirectEmpty.Store(len(m) == 0 && len(patterns) == 0)
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
		logger.Warn("ReloadRedirects skipped: store is nil or closed")
		return nil
	}

	m, err := db.LoadRedirectsToMap(gStore.Model)
	if err != nil {
		logger.Error("error loading redirects: %v", err)
		return err
	}

	redirectMap := make(map[string]RedirectRule, len(m))
	patterns := make([]compiledRedirectRule, 0, len(m))
	var invalidRuleErrors []error
	for from, redirect := range m {
		if redirect_rule.HasPattern(from) || redirect_rule.HasPattern(redirect.To) {
			rule, compileErr := redirect_rule.Compile(from, redirect.To)
			if compileErr != nil {
				invalidRuleErrors = append(invalidRuleErrors, fmt.Errorf("from=%s to=%s: %w", from, redirect.To, compileErr))
				continue
			}
			patterns = append(patterns, compiledRedirectRule{
				rule:   rule,
				status: redirect.Status,
			})
			continue
		}

		redirectMap[from] = RedirectRule{
			To:     redirect.To,
			Status: redirect.Status,
		}
	}
	if len(invalidRuleErrors) > 0 {
		return errors.Join(invalidRuleErrors...)
	}

	if len(patterns) > 1 {
		sortCompiledRedirectRules(patterns)
	}

	storeRedirectMap(redirectMap, patterns)
	logger.Info("redirects loaded successfully: exact=%d pattern=%d", len(redirectMap), len(patterns))
	return nil
}

func GetRedirect(path string) (RedirectRule, bool) {
	m, ok := Redirects.Load().(map[string]RedirectRule)
	if !ok {
		logger.Error("redirects atomic value has unexpected type, returning empty")
		return RedirectRule{}, false
	}

	redirect, exists := m[path]
	if exists {
		return redirect, true
	}

	normalizedPath := normalizeRedirectLookupPath(path)
	if normalizedPath != "" && normalizedPath != path {
		redirect, exists = m[normalizedPath]
		if exists {
			return redirect, true
		}
	}

	return matchRedirectPattern(normalizedPath)
}

func GetRedirectMap() map[string]RedirectRule {
	m, ok := Redirects.Load().(map[string]RedirectRule)
	if !ok {
		logger.Error("redirects atomic value has unexpected type, returning empty map")
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

func matchRedirectPattern(path string) (RedirectRule, bool) {
	if path == "" {
		return RedirectRule{}, false
	}

	patterns, ok := RedirectPatterns.Load().([]compiledRedirectRule)
	if !ok || len(patterns) == 0 {
		return RedirectRule{}, false
	}

	for _, item := range patterns {
		target, matched := item.rule.Match(path)
		if !matched || target == "" || target == path {
			continue
		}
		return RedirectRule{
			To:     target,
			Status: item.status,
		}, true
	}
	return RedirectRule{}, false
}

func sortCompiledRedirectRules(rules []compiledRedirectRule) {
	plain := make([]redirect_rule.Rule, 0, len(rules))
	byFrom := make(map[string]compiledRedirectRule, len(rules))
	for _, item := range rules {
		plain = append(plain, item.rule)
		byFrom[item.rule.From] = item
	}
	redirect_rule.SortRules(plain)
	for i, rule := range plain {
		rules[i] = byFrom[rule.From]
	}
}
