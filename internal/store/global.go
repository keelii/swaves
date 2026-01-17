package store

import (
	"swaves/internal/db"
	"swaves/internal/types"
)

type GlobalStore struct {
	Model   *db.DB
	Session *types.SessionStore
}

func NewGlobalStore(model *db.DB, session *types.SessionStore) *GlobalStore {
	return &GlobalStore{
		Model:   model,
		Session: session,
	}
}

func (s *GlobalStore) Close() {
	s.Model.Close()
}
