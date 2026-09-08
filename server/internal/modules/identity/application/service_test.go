package application_test

import (
	"context"
	"errors"
	"fmt"
	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitytoken "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/token"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"testing"
	"time"
)

var _ = Describe("Service", func() {
	var (
		ctx      context.Context
		users    *fakeUsers
		sessions *fakeSessions
		service  *identityapp.Service
	)

	BeforeEach(func() {
		ctx = context.Background()
		users = newFakeUsers()
		sessions = newFakeSessions(users)
		tokens, err := identitytoken.NewManager("test-secret-with-more-than-thirty-two-characters", 15*time.Minute)
		Expect(err).NotTo(HaveOccurred())
		service, err = identityapp.NewService(users, sessions, tokens, 30*24*time.Hour)
		Expect(err).NotTo(HaveOccurred())
	})

	It("blocks a disabled account from signing in, refreshing or using a token", func() {
		registered, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		users.disable("alice")

		// A correct password still fails, with a reason rather than a generic
		// credential error.
		_, err = service.Login(ctx, "alice", "password123")
		Expect(err).To(MatchError(identitydomain.ErrAccountDisabled))

		// An access token already in flight stops working immediately.
		_, err = service.Authenticate(ctx, registered.AccessToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))

		// And the refresh token cannot mint a new one.
		_, err = service.Refresh(ctx, registered.RefreshToken)
		Expect(err).To(MatchError(identitydomain.ErrAccountDisabled))
	})

	It("still rejects a wrong password on a disabled account as a credential error", func() {
		_, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		users.disable("alice")
		// Reporting "disabled" to someone who does not know the password would
		// confirm that the account exists.
		_, err = service.Login(ctx, "alice", "wrong-password")
		Expect(err).To(MatchError(identitydomain.ErrInvalidCredentials))
	})

	It("validates registration in the Identity service", func() {
		_, err := service.Register(ctx, "x", "not-an-email", "short")
		Expect(err).To(MatchError(ContainSubstring("username must")))
		Expect(errors.Is(err, identitydomain.ErrInvalidInput)).To(BeTrue())
	})

	It("performs a password check even when the username does not exist", func() {
		manager, err := identitytoken.NewManager("test-secret-with-more-than-thirty-two-characters", 15*time.Minute)
		Expect(err).NotTo(HaveOccurred())
		recording := &recordingTokenManager{TokenManager: manager}
		isolated, err := identityapp.NewService(users, sessions, recording, 30*24*time.Hour)
		Expect(err).NotTo(HaveOccurred())

		_, err = isolated.Login(ctx, "missing-user", "password123")
		Expect(err).To(MatchError(identitydomain.ErrInvalidCredentials))
		Expect(recording.checkedHashes).To(Equal([]string{recording.dummyHash}))
	})

	It("creates a revocable session on registration", func() {
		result, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.AccessToken).NotTo(BeEmpty())
		Expect(result.RefreshToken).NotTo(BeEmpty())
		Expect(result.ExpiresIn).To(Equal(int64(900)))

		identity, err := service.Authenticate(ctx, result.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(identity.User.Username).To(Equal("alice"))

		Expect(service.Logout(ctx, identity)).To(Succeed())
		_, err = service.Authenticate(ctx, result.AccessToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))
	})

	It("rotates refresh tokens and rejects the previous credential", func() {
		initial, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := service.Refresh(ctx, initial.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.RefreshToken).NotTo(Equal(initial.RefreshToken))
		_, err = service.Refresh(ctx, initial.RefreshToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))
		_, err = service.Authenticate(ctx, rotated.AccessToken)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates a separate session on login and revokes every session with logout-all", func() {
		registered, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		loggedIn, err := service.Login(ctx, "alice", "password123")
		Expect(err).NotTo(HaveOccurred())
		Expect(loggedIn.RefreshToken).NotTo(Equal(registered.RefreshToken))

		firstIdentity, err := service.Authenticate(ctx, registered.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		secondIdentity, err := service.Authenticate(ctx, loggedIn.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(secondIdentity.SessionID).NotTo(Equal(firstIdentity.SessionID))

		Expect(service.LogoutAll(ctx, secondIdentity)).To(Succeed())
		_, err = service.Authenticate(ctx, registered.AccessToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))
		_, err = service.Authenticate(ctx, loggedIn.AccessToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))
	})

	It("returns the same unauthorized error for revoked, expired and unknown refresh credentials", func() {
		registered, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		identity, err := service.Authenticate(ctx, registered.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Logout(ctx, identity)).To(Succeed())
		_, err = service.Refresh(ctx, registered.RefreshToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))

		active, err := service.Login(ctx, "alice", "password123")
		Expect(err).NotTo(HaveOccurred())
		for _, record := range sessions.byID {
			record.session.ExpiresAt = time.Now().Add(-time.Minute)
		}
		_, err = service.Refresh(ctx, active.RefreshToken)
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))
		_, err = service.Refresh(ctx, "unknown-refresh-token")
		Expect(err).To(MatchError(identitydomain.ErrUnauthorized))
	})

})

type recordingTokenManager struct {
	identityapp.TokenManager

	dummyHash     string
	checkedHashes []string
}

func (m *recordingTokenManager) HashPassword(password string) (string, error) {
	hash, err := m.TokenManager.HashPassword(password)
	if m.dummyHash == "" {
		m.dummyHash = hash
	}
	return hash, err
}

func (m *recordingTokenManager) CheckPassword(hash, password string) bool {
	m.checkedHashes = append(m.checkedHashes, hash)
	return m.TokenManager.CheckPassword(hash, password)
}

type fakeUsers struct {
	byID   map[string]*identitydomain.User
	byName map[string]*identitydomain.User
	nextID int
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*identitydomain.User{}, byName: map[string]*identitydomain.User{}}
}

func (f *fakeUsers) Create(_ context.Context, username, email, passwordHash string) (*identitydomain.User, error) {
	if _, exists := f.byName[username]; exists {
		return nil, identitydomain.ErrUsernameTaken
	}
	f.nextID++
	user := &identitydomain.User{ID: fmt.Sprintf("user-%d", f.nextID), Username: username, Email: email, PasswordHash: passwordHash, Role: "user"}
	f.byID[user.ID] = user
	f.byName[user.Username] = user
	return cloneUser(user), nil
}

func (f *fakeUsers) ByUsername(_ context.Context, username string) (*identitydomain.User, error) {
	user := f.byName[username]
	if user == nil {
		return nil, identitydomain.ErrUserNotFound
	}
	return cloneUser(user), nil
}

// disable blocks an account the way the administration console does.
func (f *fakeUsers) disable(username string) {
	if user := f.byName[username]; user != nil {
		blocked := time.Now()
		user.DisabledAt = &blocked
		user.DisabledReason = "spam"
	}
}

func (f *fakeUsers) ByID(_ context.Context, id string) (*identitydomain.User, error) {
	user := f.byID[id]
	if user == nil {
		return nil, identitydomain.ErrUserNotFound
	}
	return cloneUser(user), nil
}

type fakeSessionRecord struct {
	session *identitydomain.Session
	hash    string
	revoked bool
}

type fakeSessions struct {
	users  *fakeUsers
	byID   map[string]*fakeSessionRecord
	byHash map[string]*fakeSessionRecord
	nextID int
}

func newFakeSessions(users *fakeUsers) *fakeSessions {
	return &fakeSessions{users: users, byID: map[string]*fakeSessionRecord{}, byHash: map[string]*fakeSessionRecord{}}
}

func (f *fakeSessions) Create(_ context.Context, userID string, refreshHash []byte, expiresAt time.Time) (*identitydomain.Session, error) {
	f.nextID++
	session := &identitydomain.Session{ID: fmt.Sprintf("session-%d", f.nextID), UserID: userID, ExpiresAt: expiresAt}
	record := &fakeSessionRecord{session: session, hash: string(refreshHash)}
	f.byID[session.ID] = record
	f.byHash[record.hash] = record
	copy := *session
	return &copy, nil
}

func (f *fakeSessions) Rotate(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time) (*identitydomain.Session, *identitydomain.User, error) {
	record := f.byHash[string(oldHash)]
	if record == nil || record.revoked || time.Now().After(record.session.ExpiresAt) {
		return nil, nil, identitydomain.ErrUnauthorized
	}
	delete(f.byHash, record.hash)
	record.hash = string(newHash)
	record.session.ExpiresAt = expiresAt
	f.byHash[record.hash] = record
	session := *record.session
	user, err := f.users.ByID(ctx, session.UserID)
	return &session, user, err
}

func (f *fakeSessions) ActiveUser(ctx context.Context, sessionID, userID string) (*identitydomain.User, error) {
	record := f.byID[sessionID]
	active := record != nil && !record.revoked && record.session.UserID == userID && time.Now().Before(record.session.ExpiresAt)
	if !active {
		return nil, identitydomain.ErrUnauthorized
	}
	return f.users.ByID(ctx, userID)
}

func (f *fakeSessions) Revoke(_ context.Context, sessionID, userID string) error {
	record := f.byID[sessionID]
	if record == nil || record.session.UserID != userID {
		return identitydomain.ErrUnauthorized
	}
	record.revoked = true
	return nil
}

func (f *fakeSessions) RevokeAll(_ context.Context, userID string) error {
	for _, record := range f.byID {
		if record.session.UserID == userID {
			record.revoked = true
		}
	}
	return nil
}

func cloneUser(user *identitydomain.User) *identitydomain.User {
	copy := *user
	return &copy
}

func TestIdentityService(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Identity Service Suite")
}
