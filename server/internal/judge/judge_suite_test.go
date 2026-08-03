package judge

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RimuruChan/Vertex/server/internal/database"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var integrationDB *database.DB

func TestJudge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Judge Suite")
}

var _ = BeforeSuite(func() {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		return
	}
	migrationsPath, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	Expect(err).NotTo(HaveOccurred())
	Expect(database.RunMigrations(databaseURL, migrationsPath)).To(Succeed())
	integrationDB, err = database.NewDB(context.Background(), databaseURL)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if integrationDB != nil {
		integrationDB.Close()
	}
})
