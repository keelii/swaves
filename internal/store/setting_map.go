package store

import (
	"log"
	"swaves/internal/db"
	"sync/atomic"
)

var Settings atomic.Value // 存储 map[string]string

func LoadSettings(dbx *db.DB) {
	m, err := db.LoadSettingsToMap(dbx)
	if err != nil {
		log.Fatal("Error loading settings: ", err)
	}

	Settings.Store(m) // 原子替换全局 map
	//for k := range m {
	//	fmt.Println(k)
	//}

	log.Printf("Settings loaded successfully [%d]\n", len(m))
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
func GetSettingMap() map[string]interface{} {
	s, ok := Settings.Load().(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return s
}
