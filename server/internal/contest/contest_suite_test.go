package contest_test

import (
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// integrationDB is non-nil only when TEST_DATABASE_URL is configured. The
// scoring specs that need real SQL skip without it; the pure ones always run.
func TestContest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contest Suite")
}

var integrationDB *database.DB
var releaseSuite = func() {}

var _ = BeforeSuite(func(ctx SpecContext) {
	var err error
	integrationDB, releaseSuite, err = dbtest.Shared(ctx)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	releaseSuite()
})
