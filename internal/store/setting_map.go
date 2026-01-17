package store

import (
	"log"
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
func GetSettingMap() map[string]string {
	s, ok := Settings.Load().(map[string]string)
	if !ok {
		return map[string]string{}
	}
	return s
}
