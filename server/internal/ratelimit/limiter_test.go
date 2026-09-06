package ratelimit

import (
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Limiter", func() {
	It("isolates keys and restores them after the window", func() {
		now := time.Unix(1_800_000_000, 0)
		limiter := New(8)
		limiter.now = func() time.Time { return now }

		Expect(limiter.Allow("alice", 2, time.Minute)).To(BeTrue())
		Expect(limiter.Allow("alice", 2, time.Minute)).To(BeTrue())
		Expect(limiter.Allow("alice", 2, time.Minute)).To(BeFalse())
		Expect(limiter.Allow("bob", 2, time.Minute)).To(BeTrue())

		now = now.Add(time.Minute)
		Expect(limiter.Allow("alice", 2, time.Minute)).To(BeTrue())
	})

	It("fails closed at capacity without evicting blocked keys", func() {
		now := time.Unix(1_800_000_000, 0)
		limiter := New(2)
		limiter.now = func() time.Time { return now }

		Expect(limiter.Allow("active", 1, time.Hour)).To(BeTrue())
		Expect(limiter.Allow("old", 1, time.Hour)).To(BeTrue())
		Expect(limiter.Allow("new", 1, time.Hour)).To(BeFalse())
		Expect(limiter.entries).To(HaveLen(2))
		Expect(limiter.Allow("active", 1, time.Hour)).To(BeFalse())
		Expect(limiter.Allow("old", 1, time.Hour)).To(BeFalse())
		Expect(limiter.entries).To(HaveLen(2))
	})

	It("cleans expired keys before admitting a new one", func() {
		now := time.Unix(1_800_000_000, 0)
		limiter := New(2)
		limiter.now = func() time.Time { return now }
		Expect(limiter.Allow("a", 1, time.Second)).To(BeTrue())
		Expect(limiter.Allow("b", 1, time.Second)).To(BeTrue())

		now = now.Add(time.Second)
		Expect(limiter.Allow("c", 1, time.Second)).To(BeTrue())
		Expect(limiter.entries).To(HaveLen(1))
	})

	It("uses RemoteAddr and ignores untrusted forwarding headers", func() {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = "192.0.2.10:4321"
		request.Header.Set("X-Forwarded-For", "203.0.113.9")
		Expect(ClientIdentity(request)).To(Equal("192.0.2.10"))
	})

	It("stores fixed-size scoped keys for arbitrary identities", func() {
		login := Key("login", strings.Repeat("x", 16<<10))
		register := Key("register", strings.Repeat("x", 16<<10))
		Expect(login).To(HaveLen(64))
		Expect(register).To(HaveLen(64))
		Expect(login).NotTo(Equal(register))
	})
})
