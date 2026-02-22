package store

import (
	"strconv"
	"strings"
	"swaves/internal/db"
	"swaves/internal/logger"
	"sync/atomic"
)

var Settings atomic.Value

func InitSettings(gStore *GlobalStore) {
	if err := ReloadSettings(gStore); err != nil {
		logger.Fatal("initial settings load failed: %v", err)
	}

	// 只注册一次回调
	db.OnDatabaseChanged = func(tableName db.TableName, kind db.TableOp) {
		if tableName != db.TableSettings {
			return
		}

		if kind != db.TableOpInsert && kind != db.TableOpUpdate && kind != db.TableOpDelete {
			return
		}

		if err := ReloadSettings(gStore); err != nil {
			logger.Error("reload settings failed: %v", err)
		}
	}
}

func ReloadSettings(gStore *GlobalStore) error {
	m, err := db.LoadSettingsToMap(gStore.Model)
	if err != nil {
		logger.Error("error loading settings: %v", err)
		return err
	}

	Settings.Store(m)
	logger.Info("settings loaded successfully: count=%d", len(m))
	return nil
}

func GetSetting(code string) string {
	s, ok := Settings.Load().(map[string]string)
	if !ok {
		logger.Error("error converting settings to map[string]string")
		return ""
	}
	val, exists := s[code]
	if !exists {
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
		return map[string]string{}
	}
	return s
}
