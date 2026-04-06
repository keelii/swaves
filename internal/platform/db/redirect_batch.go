package db

import (
	"database/sql"
	"time"
)

func insertRedirectTx(tx *sql.Tx, r Redirect) error {
	status := r.Status
	if status == 0 {
		status = 301
	}

	createdAt := r.CreatedAt
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	updatedAt := r.UpdatedAt
	if updatedAt == 0 {
		updatedAt = createdAt
	}

	_, err := tx.Exec(
		`INSERT INTO `+string(TableRedirects)+` (
			from_path, to_path, status, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		r.From,
		r.To,
		status,
		r.Enabled,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return WrapInternalErr("insertRedirectTx", err)
	}
	return nil
}

func CreateRedirectBatch(db *DB, redirects []Redirect) error {
	if len(redirects) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return WrapInternalErr("CreateRedirectBatch.Begin", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	for _, redirect := range redirects {
		if err := insertRedirectTx(tx, redirect); err != nil {
			return WrapInternalErr("CreateRedirectBatch.insert", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return WrapInternalErr("CreateRedirectBatch.Commit", err)
	}
	tx = nil
	return nil
}
