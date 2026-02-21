package store

import (
	"log"
	"strconv"
	"strings"
	"swaves/internal/db"
	"sync/atomic"
)

var Settings atomic.Value

func InitSettings(gStore *GlobalStore) {
	if err := ReloadSettings(gStore); err != nil {
		log.Fatal("initial settings load failed:", err)
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
			log.Println("reload settings failed:", err)
		}
	}
}

func ReloadSettings(gStore *GlobalStore) error {
	m, err := db.LoadSettingsToMap(gStore.Model)
	if err != nil {
		log.Println("Error loading settings: ", err)
		return err
	}

	Settings.Store(m)
	log.Printf("Settings loaded successfully [%d]\n", len(m))
	return nil
}

func GetSetting(code string) string {
	s, ok := Settings.Load().(map[string]string)
	if !ok {
		log.Println("Error converting Settings to map[string]string")
		return ""
	}
	val, exists := s[code]
	if !exists {
		log.Println("No settings found for code:", code)
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
		log.Printf("parse int setting %s=%q failed: %v", code, value, err)
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
