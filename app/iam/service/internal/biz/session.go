package biz

import (
	"context"
	"errors"
	"fmt"
	"time"

	userpb "github.com/Servora-Kit/plateau/api/gen/go/iam/user/v1"
)

var (
	ErrSessionRevoked = errors.New("session revoked")
	ErrUserDisabled   = errors.New("user disabled")
	ErrUserNotActive  = errors.New("user not active")
)

// LoginSession 是可撤销的认证事件；浏览器令牌和期限由 SCS 管理。
type LoginSession struct {
	ID        string
	UserID    string
	AuthTime  time.Time
	RevokedAt *time.Time
}

// SessionRepo 的撤销方法原子更新登录及其关联 OAuth 状态。
type SessionRepo interface {
	Create(context.Context, string, time.Time) (*LoginSession, error)
	Find(context.Context, string) (*LoginSession, error)
	Revoke(context.Context, string, time.Time) error
}

type SessionUsecase struct {
	users    UserRepo
	sessions SessionRepo
	now      func() time.Time
}

func NewSessionUsecase(users UserRepo, sessions SessionRepo) (*SessionUsecase, error) {
	if users == nil || sessions == nil {
		return nil, fmt.Errorf("session: repository dependency is nil")
	}
	return &SessionUsecase{users: users, sessions: sessions, now: time.Now}, nil
}

func (uc *SessionUsecase) Create(ctx context.Context, userID string) (*LoginSession, error) {
	if userID == "" {
		return nil, ErrUnauthenticated
	}
	return uc.sessions.Create(ctx, userID, uc.now())
}

// Resolve 只接受从已装载 SCS 数据读取的登录引用，并查询当前身份事实。
func (uc *SessionUsecase) Resolve(ctx context.Context, loginID string) (*userpb.User, *LoginSession, error) {
	if loginID == "" {
		return nil, nil, ErrUnauthenticated
	}
	login, err := uc.sessions.Find(ctx, loginID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, ErrSessionRevoked
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find login: %w", err)
	}
	if login == nil || login.ID == "" || login.UserID == "" || login.RevokedAt != nil {
		return nil, nil, ErrSessionRevoked
	}
	user, err := uc.users.Get(ctx, login.UserID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, ErrSessionRevoked
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find login user: %w", err)
	}
	if user.GetStatus() == userpb.UserStatus_USER_STATUS_DISABLED {
		return nil, nil, ErrUserDisabled
	}
	if user.GetStatus() != userpb.UserStatus_USER_STATUS_ACTIVE || !user.GetEmailVerified() {
		return nil, nil, ErrUserNotActive
	}
	return user, login, nil
}

func (uc *SessionUsecase) Logout(ctx context.Context, loginID string) error {
	if loginID == "" {
		return ErrUnauthenticated
	}
	return uc.sessions.Revoke(ctx, loginID, uc.now())
}
