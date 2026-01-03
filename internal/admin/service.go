package admin

import (
	"errors"

	"swaves/internal/db"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidPassword = errors.New("invalid password")

type Service struct {
	DB *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{DB: db}
}

func (a *Service) CheckPassword(raw string) error {
	cfg, err := db.GetConfig(a.DB)
	if err != nil {
		return err
	}

	return bcrypt.CompareHashAndPassword(
		[]byte(cfg.AdminPasswordHash),
		[]byte(raw),
	)
}
