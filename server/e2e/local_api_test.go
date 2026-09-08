package e2e

import (
	"context"
	"fmt"
	authoringapp "github.com/RimuruChan/Vertex/server/internal/modules/authoring/application"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/modules/authoring/infrastructure/postgres"
	authoringhandler "github.com/RimuruChan/Vertex/server/internal/modules/authoring/transport/http"
	consoleapp "github.com/RimuruChan/Vertex/server/internal/modules/console/application"
	consolepg "github.com/RimuruChan/Vertex/server/internal/modules/console/infrastructure/postgres"
	consolehttp "github.com/RimuruChan/Vertex/server/internal/modules/console/transport/http"
	contestapp "github.com/RimuruChan/Vertex/server/internal/modules/contest/application"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	contesthttp "github.com/RimuruChan/Vertex/server/internal/modules/contest/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	identitytoken "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/token"
	identityhttp "github.com/RimuruChan/Vertex/server/internal/modules/identity/transport/http"
	judgeapp "github.com/RimuruChan/Vertex/server/internal/modules/judge/application"
	judgepg "github.com/RimuruChan/Vertex/server/internal/modules/judge/infrastructure/postgres"
	judgehttp "github.com/RimuruChan/Vertex/server/internal/modules/judge/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	problemapp "github.com/RimuruChan/Vertex/server/internal/modules/problem/application"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	problemhttp "github.com/RimuruChan/Vertex/server/internal/modules/problem/transport/http"
	publicidpg "github.com/RimuruChan/Vertex/server/internal/modules/publicid/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/ratelimit"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/modules/submission/application"
	submissionstore "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/modules/submission/transport/http"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	tenancyhttp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/transport/http"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// This transport opens no socket and starts no service process. It exercises
// real routing, authentication, persistence and artifact I/O, not deployment.
type localAPITransport struct{ handler http.Handler }

func (t localAPITransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Host != "vertex-api.test" {
		return nil, fmt.Errorf("unexpected integration target")
	}
	copy := request.Clone(request.Context())
	copy.RequestURI = request.URL.RequestURI()
	copy.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	t.handler.ServeHTTP(response, copy)
	return response.Result(), nil
}

func TestDomainAPIIntegration(t *testing.T) {
	ctx := context.Background()
	db, release, err := dbtest.Shared(ctx, "../migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if db == nil {
		t.Skip("TEST_DATABASE_URL not set")
	}
	if err := dbtest.Reset(ctx, db, "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	const secret = "local-integration-only-key-not-for-production"
	if err := identitypg.BootstrapAdmin(ctx, db, "integration_admin", "integration-only-password", "integration-admin@example.test", identitytoken.HashPassword); err != nil {
		t.Fatal(err)
	}
	users := identitypg.NewUserRepository(db)
	tokens, err := identitytoken.NewManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identityapp.NewService(users, identitypg.NewSessionRepository(db), tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authMiddleware := middleware.NewAuthMiddleware(auth)
	domains := tenancyapp.NewService(tenancypg.NewRepository(db))
	root := t.TempDir()
	reader := problempg.NewQueries(db)
	problems := problemapp.NewService(reader, problempg.NewRepository(db, problemfiles.NewTestdataStorage(root)))
	contests := contestapp.NewService(contestpg.NewRepository(db), tokens)
	submissions := submissionapp.NewService(submissionstore.NewRepository(db), reader, contests, nil, nil)
	builds, err := authoringapp.NewService(authoringpg.NewPackageRepository(db), authoringpg.NewBuildRepository(db), authoringfiles.NewTestdataPublisher(root), authoringapp.NewDispatcher(1), 2*time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := judgeapp.NewService(judgepg.NewJobRepository(db), judgeapp.NewDispatcher(1), time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.Router(httpapi.Dependencies{
		Auth: identityhttp.NewAuthHandler(auth, identityhttp.AuthCookieConfig{Lifetime: time.Hour}, identityhttp.AuthRateLimits{}), Health: httpapi.NewHealthHandler(db.Pool.PingContext),
		Domains: tenancyhttp.NewHandler(domains), PublicIDs: publicidpg.NewResolver(db), ResolveDomain: middleware.ResolveDomain(domains),
		Problems: problemhttp.NewProblemHandler(problems), AdminProblems: problemhttp.NewAdminProblemHandler(problems),
		Contests: contesthttp.NewContestHandler(contests, ratelimit.Policy{}), Submissions: submissionhandler.NewSubmissionHandler(submissions),
		AdminPackages: authoringhandler.NewPackageHandler(builds), Builds: authoringhandler.NewBuildHandler(builds, authoringhandler.DefaultBuildLimits()),
		Console: consolehttp.NewConsoleHandler(consoleapp.NewService(consolepg.NewRepository(db))), Judge: judgehttp.NewJudgeHandler(jobs),
		OptionalAuth: authMiddleware.Optional(), RequireAuth: authMiddleware.Require(), RequireAdmin: middleware.RequireAdmin(), RequireJudge: middleware.RequireJudgeService(secret),
	})
	priorTransport := http.DefaultTransport
	http.DefaultTransport = localAPITransport{handler: router}
	t.Cleanup(func() { http.DefaultTransport = priorTransport })
	t.Setenv("E2E_BASE_URL", "http://vertex-api.test")
	t.Setenv("E2E_JUDGE_API_TOKEN", secret)
	t.Setenv("E2E_ADMIN_USER", "integration_admin")
	t.Setenv("E2E_ADMIN_PASS", "integration-only-password")
	t.Run("AuthLifecycle", TestEndToEndAuthLifecycle)
	t.Run("AdminConsole", TestEndToEndAdminConsole)
	t.Run("DomainWorkflow", TestEndToEndDomainWorkflow)
	t.Run("DomainProtocol", TestEndToEndDomainProtocol)
}
