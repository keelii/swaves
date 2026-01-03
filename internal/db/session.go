package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

type AdminSessionStorage struct {
	DB *DB
}

func (s *AdminSessionStorage) Get(key string) ([]byte, error) {
	fmt.Println("Getting session", key)
	var expiresAt int64
	err := s.DB.QueryRow(`SELECT expires_at FROM admin_sessions WHERE sid=?`, key).Scan(&expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fiber.ErrNotFound
		}
		return nil, err
	}
	if time.Now().Unix() > expiresAt {
		_ = s.Delete(key)
		return nil, fiber.ErrNotFound
	}
	return []byte(key), nil
}

func (s *AdminSessionStorage) Set(key string, val []byte, exp time.Duration) error {
	fmt.Println("setting key:", key)
	now := time.Now().Unix()
	expires := now + int64(exp.Seconds())
	_, err := s.DB.Exec(`
        INSERT INTO admin_sessions(sid, expires_at, created_at)
        VALUES(?, ?, ?)
        ON CONFLICT(sid) DO UPDATE SET expires_at=excluded.expires_at
    `, string(val), expires, now)
	return err
}

func (s *AdminSessionStorage) Delete(key string) error {
	fmt.Println("deleting key:", key)
	_, err := s.DB.Exec(`DELETE FROM admin_sessions WHERE sid=?`, key)
	return err
}

func (s *AdminSessionStorage) Reset() error {
	fmt.Println("resetting sessions")
	_, err := s.DB.Exec(`DELETE FROM admin_sessions`)
	return err
}

func (s *AdminSessionStorage) Close() error {
	fmt.Println("closing sessions")
	return nil
}
