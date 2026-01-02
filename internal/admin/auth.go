package admin

import (
	"errors"

	"golang.org/x/crypto/bcrypt"

	"swaves/internal/db"
)

var ErrInvalidPassword = errors.New("invalid password")

func Authenticate(dbConn *db.DB, password string) error {
	cfg, err := db.GetConfig(dbConn)
	if err != nil {
		return err
	}

	if cfg.AdminPasswordHash == "" {
		return errors.New("admin password not set")
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(cfg.AdminPasswordHash),
		[]byte(password),
	); err != nil {
		return ErrInvalidPassword
	}

	return nil
}
