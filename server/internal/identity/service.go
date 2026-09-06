package identity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid username or password")
)

var (
	usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)
	emailPattern    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

const (
	maxEmailBytes    = 254
	maxPasswordBytes = 72 // bcrypt rejects longer inputs.
	dummyPassword    = "vertex-invalid-credential-sentinel"
)

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

type UserRepository interface {
	Create(ctx context.Context, username, email, passwordHash string) (*User, error)
	ByUsername(ctx context.Context, username string) (*User, error)
	ByID(ctx context.Context, id string) (*User, error)
}

type SessionRepository interface {
	Create(ctx context.Context, userID string, refreshHash []byte, expiresAt time.Time) (*Session, error)
	Rotate(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time) (*Session, *User, error)
	ActiveUser(ctx context.Context, sessionID, userID string) (*User, error)
	Revoke(ctx context.Context, sessionID, userID string) error
	RevokeAll(ctx context.Context, userID string) error
}

type TokenManager interface {
	AccessTTL() time.Duration
	IssueAccess(userID, sessionID, username, role string) (string, error)
	ParseAccess(raw string) (*AccessClaims, error)
	NewRefreshToken() (string, []byte, error)
	HashRefreshToken(raw string) []byte
	HashPassword(password string) (string, error)
	CheckPassword(hash, password string) bool
}

type Result struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	User         *User
}

type Identity struct {
	User      *User
	SessionID string
	TokenID   string
}

type Service struct {
	users      UserRepository
	sessions   SessionRepository
	tokens     TokenManager
	refreshTTL time.Duration
	dummyHash  string
	now        func() time.Time
}

func NewService(users UserRepository, sessions SessionRepository, tokens TokenManager, refreshTTL time.Duration) (*Service, error) {
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
	if !usernamePattern.MatchString(username) {
		return nil, &ValidationError{Message: "username must be 3-32 chars of letters, digits or underscore"}
	}
	if len(email) > maxEmailBytes || !emailPattern.MatchString(email) {
		return nil, &ValidationError{Message: "invalid email"}
	}
	if len(password) < 6 {
		return nil, &ValidationError{Message: "password must be at least 6 chars"}
	}
	if len(password) > maxPasswordBytes {
		return nil, &ValidationError{Message: "password must be at most 72 bytes"}
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
	if len(password) > maxPasswordBytes {
		return nil, ErrInvalidCredentials
	}
	user, err := s.users.ByUsername(ctx, strings.TrimSpace(username))
	if err != nil && !errors.Is(err, ErrUserNotFound) {
		return nil, err
	}
	found := err == nil && user != nil
	hash := s.dummyHash
	if found {
		hash = user.PasswordHash
	}
	if !s.tokens.CheckPassword(hash, password) || !found {
		return nil, ErrInvalidCredentials
	}
	// The password check runs first on purpose: reporting "disabled" to someone
	// who does not know the password would leak that the account exists.
	if user.Disabled() {
		return nil, ErrAccountDisabled
	}
	return s.createSession(ctx, user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Result, error) {
	if refreshToken == "" {
		return nil, ErrUnauthorized
	}
	newRaw, newHash, err := s.tokens.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	session, user, err := s.sessions.Rotate(ctx, s.tokens.HashRefreshToken(refreshToken), newHash, s.now().Add(s.refreshTTL))
	if err != nil {
		return nil, ErrUnauthorized
	}
	// Blocking an account revokes its sessions, but a direct database edit
	// would not; checking here keeps the block effective either way.
	if user.Disabled() {
		return nil, ErrAccountDisabled
	}
	return s.issue(user, session.ID, newRaw)
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (*Identity, error) {
	claims, err := s.tokens.ParseAccess(accessToken)
	if err != nil {
		return nil, ErrUnauthorized
	}
	user, err := s.sessions.ActiveUser(ctx, claims.SessionID, claims.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	// An access token already in flight stops working as soon as the account is
	// blocked, without waiting for it to expire.
	if user.Disabled() {
		return nil, ErrUnauthorized
	}
	return &Identity{User: user, SessionID: claims.SessionID, TokenID: claims.TokenID}, nil
}

func (s *Service) Logout(ctx context.Context, identity *Identity) error {
	if identity == nil || identity.User == nil {
		return ErrUnauthorized
	}
	return s.sessions.Revoke(ctx, identity.SessionID, identity.User.ID)
}

func (s *Service) LogoutAll(ctx context.Context, identity *Identity) error {
	if identity == nil || identity.User == nil {
		return ErrUnauthorized
	}
	return s.sessions.RevokeAll(ctx, identity.User.ID)
}

func (s *Service) createSession(ctx context.Context, user *User) (*Result, error) {
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

func (s *Service) issue(user *User, sessionID, refreshToken string) (*Result, error) {
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
