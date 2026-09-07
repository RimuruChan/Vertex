package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/authoring"
	authoringhandler "github.com/RimuruChan/Vertex/server/internal/authoring/handler"
	"github.com/RimuruChan/Vertex/server/internal/console"
	consolehandler "github.com/RimuruChan/Vertex/server/internal/console/handler"
	contestapp "github.com/RimuruChan/Vertex/server/internal/contest"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	domainhandler "github.com/RimuruChan/Vertex/server/internal/domain/handler"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	identityhandler "github.com/RimuruChan/Vertex/server/internal/identity/handler"
	"github.com/RimuruChan/Vertex/server/internal/judge"
	judgehandler "github.com/RimuruChan/Vertex/server/internal/judge/handler"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/submission"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/handler"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
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
	if err := identity.BootstrapAdmin(ctx, db, "integration_admin", "integration-only-password", "integration-admin@example.test", identity.HashPassword); err != nil {
		t.Fatal(err)
	}
	users := identity.NewUserStore(db)
	tokens, err := identity.NewManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := identity.NewService(users, identity.NewSessionStore(db), tokens, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authMiddleware := middleware.NewAuthMiddleware(auth)
	domains := domain.NewService(domain.NewStore(db))
	root := t.TempDir()
	reader := problem.NewProblemStore(db)
	problems := problem.NewService(reader, problem.NewProblemAdminStore(db, root))
	contests := contestapp.NewService(contestapp.NewContestStore(db), tokens)
	submissions := submissionapp.NewService(submissionapp.NewSubmissionStore(db), reader, contests, nil, nil)
	builds, err := authoring.NewService(authoring.NewPackageStore(db), authoring.NewBuildStore(db), authoring.NewTestdataPublisher(root), authoring.NewDispatcher(1), 2*time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := judge.NewService(judge.NewJudgeJobStore(db), judge.NewDispatcher(1), time.Minute, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.Router(httpapi.Dependencies{
		Auth: identityhandler.NewAuthHandler(auth, identityhandler.AuthCookieConfig{Lifetime: time.Hour}, identityhandler.AuthRateLimits{}), Health: httpapi.NewHealthHandler(db.Pool.PingContext),
		Domains: domainhandler.NewHandler(domains), PublicIDs: publicid.NewStore(db), ResolveDomain: middleware.ResolveDomain(domains),
		Problems: problemhandler.NewProblemHandler(problems), AdminProblems: problemhandler.NewAdminProblemHandler(problems),
		Contests: contesthandler.NewContestHandler(contests, ratelimit.Policy{}), Submissions: submissionhandler.NewSubmissionHandler(submissions),
		AdminPackages: authoringhandler.NewPackageHandler(builds), Builds: authoringhandler.NewBuildHandler(builds, authoringhandler.DefaultBuildLimits()),
		Console: consolehandler.NewConsoleHandler(console.NewService(console.NewConsoleStore(db))), Judge: judgehandler.NewJudgeHandler(jobs),
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
