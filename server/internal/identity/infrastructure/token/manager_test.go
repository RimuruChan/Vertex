package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager", func() {
	const secret = "test-secret-with-more-than-thirty-two-characters"

	It("issues access tokens with the required identity and session claims", func() {
		manager, err := NewManager(secret, 15*time.Minute)
		Expect(err).NotTo(HaveOccurred())
		raw, err := manager.IssueAccess("user-1", "session-1", "alice", "user")
		Expect(err).NotTo(HaveOccurred())
		issued := &Claims{}
		parsed, err := jwt.ParseWithClaims(raw, issued, func(*jwt.Token) (any, error) {
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		Expect(err).NotTo(HaveOccurred())
		Expect(parsed.Valid).To(BeTrue())
		Expect(issued.Issuer).To(Equal(issuer))
		Expect(issued.Audience).To(ContainElement(audience))
		Expect(issued.Subject).To(Equal("user-1"))
		Expect(issued.SessionID).To(Equal("session-1"))
		Expect(issued.ID).NotTo(BeEmpty())
		Expect(issued.Username).To(Equal("alice"))
		Expect(issued.Role).To(Equal("user"))
		Expect(issued.TokenType).To(Equal("access"))
		Expect(issued.NotBefore).NotTo(BeNil())
		Expect(issued.ExpiresAt).NotTo(BeNil())

		claims, err := manager.ParseAccess(raw)
		Expect(err).NotTo(HaveOccurred())
		Expect(claims.UserID).To(Equal("user-1"))
		Expect(claims.SessionID).To(Equal("session-1"))
		Expect(claims.TokenID).NotTo(BeEmpty())
	})

	It("rejects a token signed with another HMAC algorithm", func() {
		manager, err := NewManager(secret, 15*time.Minute)
		Expect(err).NotTo(HaveOccurred())
		now := time.Now()
		claims := Claims{
			SessionID: "session-1",
			TokenType: "access",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: issuer, Audience: jwt.ClaimStrings{audience}, Subject: "user-1", ID: "token-1",
				IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
			},
		}
		raw, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(secret))
		Expect(err).NotTo(HaveOccurred())
		_, err = manager.ParseAccess(raw)
		Expect(err).To(MatchError(ErrInvalidToken))
	})

	It("rejects expired access tokens", func() {
		manager, err := NewManager(secret, time.Second)
		Expect(err).NotTo(HaveOccurred())
		now := time.Now()
		manager.now = func() time.Time { return now }
		raw, err := manager.IssueAccess("user-1", "session-1", "alice", "user")
		Expect(err).NotTo(HaveOccurred())
		manager.now = func() time.Time { return now.Add(2 * time.Second) }
		_, err = manager.ParseAccess(raw)
		Expect(err).To(MatchError(ErrExpiredToken))
	})

	DescribeTable("rejects invalid registered or access claims",
		func(mutate func(*Claims)) {
			manager, err := NewManager(secret, 15*time.Minute)
			Expect(err).NotTo(HaveOccurred())
			now := time.Now()
			manager.now = func() time.Time { return now }
			claims := &Claims{
				SessionID: "session-1", Username: "alice", Role: "user", TokenType: "access",
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer: issuer, Audience: jwt.ClaimStrings{audience}, Subject: "user-1", ID: "token-1",
					IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
					ExpiresAt: jwt.NewNumericDate(now.Add(time.Minute)),
				},
			}
			mutate(claims)
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
			Expect(err).NotTo(HaveOccurred())
			_, err = manager.ParseAccess(raw)
			Expect(err).To(MatchError(ErrInvalidToken))
		},
		Entry("issuer", func(claims *Claims) { claims.Issuer = "another-issuer" }),
		Entry("audience", func(claims *Claims) { claims.Audience = jwt.ClaimStrings{"another-audience"} }),
		Entry("token type", func(claims *Claims) { claims.TokenType = "refresh" }),
		Entry("session ID", func(claims *Claims) { claims.SessionID = "" }),
		Entry("token ID", func(claims *Claims) { claims.ID = "" }),
		Entry("not-before", func(claims *Claims) {
			claims.NotBefore = jwt.NewNumericDate(claims.IssuedAt.Time.Add(time.Hour))
		}),
	)
})

func TestManager(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Token Manager") }
