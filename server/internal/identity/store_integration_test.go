package identity_test

import (
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	authapp "github.com/RimuruChan/Vertex/server/internal/identity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationDB *database.DB
var releaseIntegrationDB = func() {}

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseIntegrationDB, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseIntegrationDB()
})

var _ = Describe("User store against PostgreSQL", func() {
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		_, err := integrationDB.Pool.ExecContext(ctx, `TRUNCATE auth_sessions, users RESTART IDENTITY CASCADE`)
		Expect(err).NotTo(HaveOccurred())
	})

	It("loads disabled account fields by ID", func(ctx SpecContext) {
		var userID string
		Expect(integrationDB.Pool.QueryRowContext(ctx,
			`INSERT INTO users (username, email, password_hash, disabled_at, disabled_reason)
			 VALUES ('blocked', 'blocked@example.test', 'hash', now(), 'spam')
			 RETURNING id`).Scan(&userID)).To(Succeed())

		user, err := authapp.NewUserStore(integrationDB).ByID(ctx, userID)
		Expect(err).NotTo(HaveOccurred())
		Expect(user.ID).To(Equal(userID))
		Expect(user.DisabledAt).NotTo(BeNil())
		Expect(user.DisabledReason).To(Equal("spam"))
	})
})
