package session

import (
	"database/sql"
	"fmt"

	sessionpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/session/v1"
	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
)

// NewPostgres 基于 PostgreSQL 连接构造独立的 SCS 会话管理器；
// 若会话配置校验失败，立即停止后台清理协程以防泄漏。
func NewPostgres(config *sessionpb.Session, db *sql.DB) (*scs.SessionManager, func(), error) {
	if db == nil {
		return nil, nil, fmt.Errorf("session: postgres db is nil")
	}
	store := postgresstore.New(db)
	manager, err := New(config, store)
	if err != nil {
		store.StopCleanup()
		return nil, nil, err
	}
	return manager, store.StopCleanup, nil
}
