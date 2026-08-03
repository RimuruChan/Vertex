package identity_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	authapp "github.com/RimuruChan/Vertex/web/internal/identity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	var (
		ctx      context.Context
		users    *fakeUsers
		sessions *fakeSessions
		service  *authapp.Service
	)

	BeforeEach(func() {
		ctx = context.Background()
		users = newFakeUsers()
		sessions = newFakeSessions(users)
		tokens, err := authapp.NewManager("test-secret-with-more-than-thirty-two-characters", 15*time.Minute)
		Expect(err).NotTo(HaveOccurred())
		service, err = authapp.NewService(users, sessions, tokens, 30*24*time.Hour)
		Expect(err).NotTo(HaveOccurred())
	})

	It("validates registration in the Identity service", func() {
		_, err := service.Register(ctx, "x", "not-an-email", "short")
		Expect(err).To(MatchError(ContainSubstring("username must")))
		Expect(errors.Is(err, authapp.ErrInvalidInput)).To(BeTrue())
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
		Expect(err).To(MatchError(authapp.ErrUnauthorized))
	})

	It("rotates refresh tokens and rejects the previous credential", func() {
		initial, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		rotated, err := service.Refresh(ctx, initial.RefreshToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(rotated.RefreshToken).NotTo(Equal(initial.RefreshToken))
		_, err = service.Refresh(ctx, initial.RefreshToken)
		Expect(err).To(MatchError(authapp.ErrUnauthorized))
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
		Expect(err).To(MatchError(authapp.ErrUnauthorized))
		_, err = service.Authenticate(ctx, loggedIn.AccessToken)
		Expect(err).To(MatchError(authapp.ErrUnauthorized))
	})

	It("returns the same unauthorized error for revoked, expired and unknown refresh credentials", func() {
		registered, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())
		identity, err := service.Authenticate(ctx, registered.AccessToken)
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Logout(ctx, identity)).To(Succeed())
		_, err = service.Refresh(ctx, registered.RefreshToken)
		Expect(err).To(MatchError(authapp.ErrUnauthorized))

		active, err := service.Login(ctx, "alice", "password123")
		Expect(err).NotTo(HaveOccurred())
		sessions.mu.Lock()
		for _, record := range sessions.byID {
			record.session.ExpiresAt = time.Now().Add(-time.Minute)
		}
		sessions.mu.Unlock()
		_, err = service.Refresh(ctx, active.RefreshToken)
		Expect(err).To(MatchError(authapp.ErrUnauthorized))
		_, err = service.Refresh(ctx, "unknown-refresh-token")
		Expect(err).To(MatchError(authapp.ErrUnauthorized))
	})

	It("allows only one concurrent use of a refresh token", func() {
		initial, err := service.Register(ctx, "alice", "alice@example.test", "password123")
		Expect(err).NotTo(HaveOccurred())

		const attempts = 8
		errorsSeen := make(chan error, attempts)
		var wg sync.WaitGroup
		for range attempts {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				_, refreshErr := service.Refresh(ctx, initial.RefreshToken)
				errorsSeen <- refreshErr
			}()
		}
		wg.Wait()
		close(errorsSeen)

		successes := 0
		for refreshErr := range errorsSeen {
			if refreshErr == nil {
				successes++
			} else {
				Expect(refreshErr).To(MatchError(authapp.ErrUnauthorized))
			}
		}
		Expect(successes).To(Equal(1))
	})
})

type fakeUsers struct {
	mu     sync.Mutex
	byID   map[string]*authapp.User
	byName map[string]*authapp.User
	nextID int
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{byID: map[string]*authapp.User{}, byName: map[string]*authapp.User{}}
}

func (f *fakeUsers) Create(_ context.Context, username, email, passwordHash string) (*authapp.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.byName[username]; exists {
		return nil, authapp.ErrUsernameTaken
	}
	f.nextID++
	user := &authapp.User{ID: fmt.Sprintf("user-%d", f.nextID), Username: username, Email: email, PasswordHash: passwordHash, Role: "user"}
	f.byID[user.ID] = user
	f.byName[user.Username] = user
	return cloneUser(user), nil
}

func (f *fakeUsers) ByUsername(_ context.Context, username string) (*authapp.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user := f.byName[username]
	if user == nil {
		return nil, authapp.ErrUserNotFound
	}
	return cloneUser(user), nil
}

func (f *fakeUsers) ByID(_ context.Context, id string) (*authapp.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	user := f.byID[id]
	if user == nil {
		return nil, authapp.ErrUserNotFound
	}
	return cloneUser(user), nil
}

type fakeSessionRecord struct {
	session *authapp.Session
	hash    string
	revoked bool
}

type fakeSessions struct {
	mu     sync.Mutex
	users  *fakeUsers
	byID   map[string]*fakeSessionRecord
	byHash map[string]*fakeSessionRecord
	nextID int
}

func newFakeSessions(users *fakeUsers) *fakeSessions {
	return &fakeSessions{users: users, byID: map[string]*fakeSessionRecord{}, byHash: map[string]*fakeSessionRecord{}}
}

func (f *fakeSessions) Create(_ context.Context, userID string, refreshHash []byte, expiresAt time.Time) (*authapp.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	session := &authapp.Session{ID: fmt.Sprintf("session-%d", f.nextID), UserID: userID, ExpiresAt: expiresAt}
	record := &fakeSessionRecord{session: session, hash: string(refreshHash)}
	f.byID[session.ID] = record
	f.byHash[record.hash] = record
	copy := *session
	return &copy, nil
}

func (f *fakeSessions) Rotate(ctx context.Context, oldHash, newHash []byte, expiresAt time.Time) (*authapp.Session, *authapp.User, error) {
	f.mu.Lock()
	record := f.byHash[string(oldHash)]
	if record == nil || record.revoked || time.Now().After(record.session.ExpiresAt) {
		f.mu.Unlock()
		return nil, nil, authapp.ErrUnauthorized
	}
	delete(f.byHash, record.hash)
	record.hash = string(newHash)
	record.session.ExpiresAt = expiresAt
	f.byHash[record.hash] = record
	session := *record.session
	f.mu.Unlock()
	user, err := f.users.ByID(ctx, session.UserID)
	return &session, user, err
}

func (f *fakeSessions) ActiveUser(ctx context.Context, sessionID, userID string) (*authapp.User, error) {
	f.mu.Lock()
	record := f.byID[sessionID]
	active := record != nil && !record.revoked && record.session.UserID == userID && time.Now().Before(record.session.ExpiresAt)
	f.mu.Unlock()
	if !active {
		return nil, authapp.ErrUnauthorized
	}
	return f.users.ByID(ctx, userID)
}

func (f *fakeSessions) Revoke(_ context.Context, sessionID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	record := f.byID[sessionID]
	if record == nil || record.session.UserID != userID {
		return authapp.ErrUnauthorized
	}
	record.revoked = true
	return nil
}

func (f *fakeSessions) RevokeAll(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, record := range f.byID {
		if record.session.UserID == userID {
			record.revoked = true
		}
	}
	return nil
}

func cloneUser(user *authapp.User) *authapp.User {
	copy := *user
	return &copy
}
