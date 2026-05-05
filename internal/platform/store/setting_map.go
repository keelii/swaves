package store

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"sync/atomic"
)

var Settings atomic.Value
var settingEmpty atomic.Bool

func init() {
	settingEmpty.Store(false)
}

func storeSettingsMap(m map[string]string) {
	if m == nil {
		m = map[string]string{}
	}

	Settings.Store(m)
	settingEmpty.Store(len(m) == 0)
}

func InitSettings(gStore *GlobalStore) {
	if err := ReloadSettings(gStore); err != nil {
		logger.Fatal("initial settings load failed: %v", err)
	}
	registerDatabaseChangeHandler(gStore)
}

func ReloadSettings(gStore *GlobalStore) error {
	if gStore == nil || gStore.IsClosed() {
		logger.Warn("ReloadSettings skipped: store is nil or closed")
		return nil
	}

	m, err := db.LoadSettingsToMap(gStore.Model)
	if err != nil {
		logger.Error("error loading settings: %v", err)
		return err
	}

	storeSettingsMap(m)

	logger.Info("settings loaded successfully: count=%d", len(m))
	return nil
}

func GetSetting(code string) string {
	s, ok := Settings.Load().(map[string]string)
	if !ok {
		logger.Error("error converting settings to map[string]string")
		s = map[string]string{}
	}
	val, exists := s[code]
	if !exists {
		if len(s) == 0 {
			logger.Error("settings map is empty while reading code: %s", code)
		}
		logger.Warn("no settings found for code: %s", code)
	}
	return val
}

func GetSettingBool(code string, defaultValue bool) bool {
	value := strings.TrimSpace(strings.ToLower(GetSetting(code)))
	if value == "" {
		return defaultValue
	}

	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func GetSettingInt(code string, defaultValue int) int {
	value := strings.TrimSpace(GetSetting(code))
	if value == "" {
		return defaultValue
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		logger.Warn("parse int setting %s=%q failed: %v", code, value, err)
		return defaultValue
	}
	return parsed
}

func GetSettingMap() map[string]string {
	s, ok := Settings.Load().(map[string]string)
	if !ok {
		logger.Error("settings atomic value has unexpected type, returning empty map")
		return map[string]string{}
	}
	return s
}

func IsSettingEmpty() bool {
	return settingEmpty.Load()
}
