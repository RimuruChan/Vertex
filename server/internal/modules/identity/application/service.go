package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
)

const dummyPassword = "vertex-invalid-credential-sentinel"

type TokenManager interface {
	AccessTTL() time.Duration
	IssueAccess(userID, sessionID, username, role string) (string, error)
	ParseAccess(raw string) (*identitydomain.AccessClaims, error)
	NewRefreshToken() (string, []byte, error)
	HashRefreshToken(raw string) []byte
	HashPassword(password string) (string, error)
	CheckPassword(hash, password string) bool
}

type Result struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	User         *identitydomain.User
}

type Identity struct {
	User      *identitydomain.User
	SessionID string
	TokenID   string
}

type Service struct {
	users      identitydomain.UserRepository
	sessions   identitydomain.SessionRepository
	tokens     TokenManager
	refreshTTL time.Duration
	dummyHash  string
	now        func() time.Time
}

func NewService(users identitydomain.UserRepository, sessions identitydomain.SessionRepository, tokens TokenManager, refreshTTL time.Duration) (*Service, error) {
	if users == nil || sessions == nil || tokens == nil {
		return nil, errors.New("auth dependencies must not be nil")
	}
	if refreshTTL <= 0 {
		return nil, errors.New("refresh token TTL must be positive")
	}
	dummyHash, err := tokens.HashPassword(dummyPassword)
	if err != nil {
		return nil, fmt.Errorf("prepare credential check: %w", err)
	}
	return &Service{
		users: users, sessions: sessions, tokens: tokens,
		refreshTTL: refreshTTL, dummyHash: dummyHash, now: time.Now,
	}, nil
}

func (s *Service) Register(ctx context.Context, username, email, password string) (*Result, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if err := identitydomain.ValidateRegistration(username, email, password); err != nil {
		return nil, err
	}

	hash, err := s.tokens.HashPassword(password)
	if err != nil {
		return nil, err
	}
	user, err := s.users.Create(ctx, username, email, hash)
	if err != nil {
		return nil, err
	}
	return s.createSession(ctx, user)
}

func (s *Service) Login(ctx context.Context, username, password string) (*Result, error) {
	if len(password) > identitydomain.MaxPasswordBytes {
		return nil, identitydomain.ErrInvalidCredentials
	}
	user, err := s.users.ByUsername(ctx, strings.TrimSpace(username))
	if err != nil && !errors.Is(err, identitydomain.ErrUserNotFound) {
		return nil, err
	}
	found := err == nil && user != nil
	hash := s.dummyHash
	if found {
		hash = user.PasswordHash
	}
	if !s.tokens.CheckPassword(hash, password) || !found {
		return nil, identitydomain.ErrInvalidCredentials
	}
	// The password check runs first on purpose: reporting "disabled" to someone
	// who does not know the password would leak that the account exists.
	if user.Disabled() {
		return nil, identitydomain.ErrAccountDisabled
	}
	return s.createSession(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Result, error) {
	if refreshToken == "" {
		return nil, identitydomain.ErrUnauthorized
	}
	newRaw, newHash, err := s.tokens.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	session, user, err := s.sessions.Rotate(ctx, s.tokens.HashRefreshToken(refreshToken), newHash, s.now().Add(s.refreshTTL))
	if err != nil {
		return nil, identitydomain.ErrUnauthorized
	}
	// Blocking an account revokes its sessions, but a direct database edit
	// would not; checking here keeps the block effective either way.
	if user.Disabled() {
		return nil, identitydomain.ErrAccountDisabled
	}
	return s.issue(user, session.ID, newRaw)
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (*Identity, error) {
	claims, err := s.tokens.ParseAccess(accessToken)
	if err != nil {
		return nil, identitydomain.ErrUnauthorized
	}
	user, err := s.sessions.ActiveUser(ctx, claims.SessionID, claims.UserID)
	if err != nil {
		return nil, identitydomain.ErrUnauthorized
	}
	// An access token already in flight stops working as soon as the account is
	// blocked, without waiting for it to expire.
	if user.Disabled() {
		return nil, identitydomain.ErrUnauthorized
	}
	return &Identity{User: user, SessionID: claims.SessionID, TokenID: claims.TokenID}, nil
}

func (s *Service) Logout(ctx context.Context, identity *Identity) error {
	if identity == nil || identity.User == nil {
		return identitydomain.ErrUnauthorized
	}
	return s.sessions.Revoke(ctx, identity.SessionID, identity.User.ID)
}

func (s *Service) LogoutAll(ctx context.Context, identity *Identity) error {
	if identity == nil || identity.User == nil {
		return identitydomain.ErrUnauthorized
	}
	return s.sessions.RevokeAll(ctx, identity.User.ID)
}

func (s *Service) createSession(ctx context.Context, user *identitydomain.User) (*Result, error) {
	raw, hash, err := s.tokens.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	session, err := s.sessions.Create(ctx, user.ID, hash, s.now().Add(s.refreshTTL))
	if err != nil {
		return nil, err
	}
	return s.issue(user, session.ID, raw)
}

func (s *Service) issue(user *identitydomain.User, sessionID, refreshToken string) (*Result, error) {
	accessToken, err := s.tokens.IssueAccess(user.ID, sessionID, user.Username, user.Role)
	if err != nil {
		return nil, err
	}
	return &Result{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.tokens.AccessTTL() / time.Second),
		User:         user,
	}, nil
}
