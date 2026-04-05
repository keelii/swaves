package store

import (
	"swaves/internal/platform/db"
	"swaves/internal/shared/types"
	"sync/atomic"
)

type GlobalStore struct {
	Model   *db.DB
	Session *types.SessionStore
	closed  atomic.Bool
}

func NewGlobalStore(model *db.DB, session *types.SessionStore) *GlobalStore {
	return &GlobalStore{
		Model:   model,
		Session: session,
	}
}

func (s *GlobalStore) Close() {
	s.closed.Store(true)
	s.Model.Close()
}

func (s *GlobalStore) IsClosed() bool {
	if s == nil {
		return true
	}
	return s.closed.Load()
}
