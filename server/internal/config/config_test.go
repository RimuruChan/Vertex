package config_test

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	It("uses safe non-secret defaults after credentials are configured", func() {
		cfg, err := config.Parse(validLookup(nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.AccessTokenTTL).To(Equal(15 * time.Minute))
		Expect(cfg.RefreshTokenTTL).To(Equal(30 * 24 * time.Hour))
		Expect(cfg.AuthCookieSecure).To(BeFalse())
		Expect(cfg.JudgeLongPollTimeout).To(Equal(25 * time.Second))
		Expect(cfg.JudgeLeaseTTL).To(Equal(45 * time.Second))
	})

	It("requires explicit connection and authentication credentials", func() {
		_, err := config.Parse(mapLookup(nil))
		Expect(err).To(MatchError("DATABASE_URL must be explicitly configured"))

		_, err = config.Parse(mapLookup(map[string]string{"DATABASE_URL": "postgres://vertex@localhost/vertex"}))
		Expect(err).To(MatchError("JWT_SECRET must contain at least 32 characters"))

		_, err = config.Parse(mapLookup(map[string]string{
			"DATABASE_URL": "postgres://vertex@localhost/vertex",
			"JWT_SECRET":   "a-secret-with-at-least-thirty-two-characters",
		}))
		Expect(err).To(MatchError("JUDGE_API_TOKEN must contain at least 32 characters"))
	})

	It("rejects invalid durations", func() {
		_, err := config.Parse(validLookup(map[string]string{"AUTH_ACCESS_TTL": "zero"}))
		Expect(err).To(MatchError("AUTH_ACCESS_TTL must be a positive duration"))
	})

	It("rejects invalid ports and empty CORS allowlists", func() {
		_, err := config.Parse(validLookup(map[string]string{"PORT": "70000"}))
		Expect(err).To(MatchError("PORT must be an integer between 1 and 65535"))
		_, err = config.Parse(validLookup(map[string]string{"CORS_ALLOWED_ORIGINS": "   "}))
		Expect(err).To(MatchError("CORS_ALLOWED_ORIGINS must contain at least one origin"))
	})

	It("requires production secrets and secure cookies", func() {
		_, err := config.Parse(mapLookup(map[string]string{
			"APP_ENV": "production", "DATABASE_URL": "postgres://vertex@localhost/vertex",
			"JUDGE_API_TOKEN": "a-judge-token-with-at-least-thirty-two-characters",
		}))
		Expect(err).To(MatchError(ContainSubstring("JWT_SECRET")))

		cfg, err := config.Parse(validLookup(map[string]string{
			"APP_ENV": "production", "JWT_SECRET": "a-secret-with-at-least-thirty-two-characters",
			"JUDGE_API_TOKEN": "a-judge-token-with-at-least-thirty-two-characters",
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.AuthCookieSecure).To(BeTrue())

		_, err = config.Parse(validLookup(map[string]string{
			"APP_ENV": "production", "JWT_SECRET": "a-secret-with-at-least-thirty-two-characters",
			"JUDGE_API_TOKEN":    "a-judge-token-with-at-least-thirty-two-characters",
			"AUTH_COOKIE_SECURE": "false",
		}))
		Expect(err).To(MatchError("AUTH_COOKIE_SECURE must be true in production"))
	})

	It("requires complete administrator bootstrap credentials", func() {
		_, err := config.Parse(validLookup(map[string]string{"ADMIN_USERNAME": "admin"}))
		Expect(err).To(MatchError("ADMIN_USERNAME and ADMIN_PASSWORD must be configured together"))
		cfg, err := config.Parse(validLookup(map[string]string{
			"ADMIN_USERNAME": "admin", "ADMIN_PASSWORD": "secret", "ADMIN_EMAIL": "ops@example.com",
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.AdminEmail).To(Equal("ops@example.com"))
	})
})

func validLookup(overrides map[string]string) func(string) (string, bool) {
	values := map[string]string{
		"DATABASE_URL":    "postgres://vertex@localhost/vertex",
		"JWT_SECRET":      "a-secret-with-at-least-thirty-two-characters",
		"JUDGE_API_TOKEN": "a-judge-token-with-at-least-thirty-two-characters",
	}
	for key, value := range overrides {
		values[key] = value
	}
	return mapLookup(values)
}

func mapLookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}
