package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/RimuruChan/Vertex/server/docs"
	authoringapp "github.com/RimuruChan/Vertex/server/internal/authoring/application"
	authoringfiles "github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/filesystem"
	authoringpg "github.com/RimuruChan/Vertex/server/internal/authoring/infrastructure/postgres"
	authoringhandler "github.com/RimuruChan/Vertex/server/internal/authoring/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/config"
	consoleapp "github.com/RimuruChan/Vertex/server/internal/console/application"
	consolepg "github.com/RimuruChan/Vertex/server/internal/console/infrastructure/postgres"
	consolehttp "github.com/RimuruChan/Vertex/server/internal/console/transport/http"
	contentapp "github.com/RimuruChan/Vertex/server/internal/content/application"
	contentpg "github.com/RimuruChan/Vertex/server/internal/content/infrastructure/postgres"
	contenthttp "github.com/RimuruChan/Vertex/server/internal/content/transport/http"
	contestapp "github.com/RimuruChan/Vertex/server/internal/contest/application"
	contestpg "github.com/RimuruChan/Vertex/server/internal/contest/infrastructure/postgres"
	contesthttp "github.com/RimuruChan/Vertex/server/internal/contest/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/database"
	identityapp "github.com/RimuruChan/Vertex/server/internal/identity/application"
	identitypg "github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/postgres"
	identitytoken "github.com/RimuruChan/Vertex/server/internal/identity/infrastructure/token"
	identityhttp "github.com/RimuruChan/Vertex/server/internal/identity/transport/http"
	judgeapp "github.com/RimuruChan/Vertex/server/internal/judge/application"
	judgepg "github.com/RimuruChan/Vertex/server/internal/judge/infrastructure/postgres"
	judgehttp "github.com/RimuruChan/Vertex/server/internal/judge/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	problemapp "github.com/RimuruChan/Vertex/server/internal/problem/application"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/problem/infrastructure/postgres"
	problemhttp "github.com/RimuruChan/Vertex/server/internal/problem/transport/http"
	setapp "github.com/RimuruChan/Vertex/server/internal/problemset/application"
	setpg "github.com/RimuruChan/Vertex/server/internal/problemset/infrastructure/postgres"
	sethttp "github.com/RimuruChan/Vertex/server/internal/problemset/transport/http"
	profileapp "github.com/RimuruChan/Vertex/server/internal/profile/application"
	profilepg "github.com/RimuruChan/Vertex/server/internal/profile/infrastructure/postgres"
	profilehttp "github.com/RimuruChan/Vertex/server/internal/profile/transport/http"
	publicidpg "github.com/RimuruChan/Vertex/server/internal/publicid/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/submission/application"
	submissionmemory "github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/memory"
	submission "github.com/RimuruChan/Vertex/server/internal/submission/infrastructure/postgres"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/transport/http"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/tenancy/application"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/tenancy/infrastructure/postgres"
	tenancyhttp "github.com/RimuruChan/Vertex/server/internal/tenancy/transport/http"
	api "github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
)

// @title						Vertex OJ API
// @version					1.0
// @description				Public application endpoints and the authenticated Judge worker protocol.
// @BasePath					/
// @securityDefinitions.apikey	BearerAuth
// @in							header
// @name						Authorization
// @description				Prefix the access token with "Bearer ".
// @securityDefinitions.apikey	JudgeServiceAuth
// @in							header
// @name						Authorization
// @description				Internal Judge service credential using the Bearer scheme.
func main() {
	if len(os.Args) == 2 && os.Args[1] == "healthcheck" {
		if err := checkHealth(); err != nil {
			os.Exit(1)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	var db *database.DB
	for attempt := 0; attempt < 10; attempt++ {
		db, err = database.NewDB(ctx, cfg.DatabaseURL)
		if err == nil {
			break
		}
		slog.Warn("postgres not ready, retrying", "attempt", attempt+1, "error", err)
		select {
		case <-ctx.Done():
			os.Exit(1)
		case <-time.After(2 * time.Second):
		}
	}
	if db == nil {
		slog.Error("cannot connect to postgres after retries")
		os.Exit(1)
	}
	defer db.Close()

	if err := database.RunMigrations(cfg.DatabaseURL, cfg.MigrationsDir); err != nil {
		slog.Error("run migrations", "error", err)
		os.Exit(1)
	}

	if err := identitypg.BootstrapAdmin(
		ctx, db, cfg.AdminUsername, cfg.AdminPassword, cfg.AdminEmail, identitytoken.HashPassword,
	); err != nil {
		slog.Error("bootstrap administrator", "error", err)
		os.Exit(1)
	}

	users := identitypg.NewUserRepository(db)
	submissions := submission.NewRepository(db)
	problems := problempg.NewQueries(db)
	contests := contestpg.NewRepository(db)
	editorials := contentpg.NewEditorialRepository(db)
	discussions := contentpg.NewDiscussionRepository(db)
	tokenManager, err := identitytoken.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		slog.Error("configure token manager", "error", err)
		os.Exit(1)
	}
	authService, err := identityapp.NewService(users, identitypg.NewSessionRepository(db), tokenManager, cfg.RefreshTokenTTL)
	if err != nil {
		slog.Error("configure authentication", "error", err)
		os.Exit(1)
	}
	abuseLimiter := ratelimit.New(cfg.RateLimitMaxKeys)
	authHandler := identityhttp.NewAuthHandler(authService, identityhttp.AuthCookieConfig{
		Secure: cfg.AuthCookieSecure, Domain: cfg.AuthCookieDomain, Lifetime: cfg.RefreshTokenTTL,
	}, identityhttp.AuthRateLimits{
		Login: ratelimit.Policy{
			Limiter: abuseLimiter, Limit: cfg.LoginRateLimit, Window: cfg.RateLimitWindow,
		},
		LoginClient: ratelimit.Policy{
			Limiter: abuseLimiter, Limit: cfg.LoginClientRateLimit, Window: cfg.RateLimitWindow,
		},
		Register: ratelimit.Policy{
			Limiter: abuseLimiter, Limit: cfg.RegisterRateLimit, Window: cfg.RateLimitWindow,
		},
	})
	authMiddleware := middleware.NewAuthMiddleware(authService)
	// Wake signals are coalesced; successful claims cascade one waiter at a time.
	judgeDispatcher := judgeapp.NewDispatcher(1)
	go judgeDispatcher.RunFallback(ctx, 5*time.Second)
	go listenForJudgeJobs(ctx, db, judgeDispatcher)
	judgeService, err := judgeapp.NewService(judgepg.NewJobRepository(db), judgeDispatcher, cfg.JudgeLeaseTTL, cfg.JudgeLongPollTimeout)
	if err != nil {
		slog.Error("configure judge service", "error", err)
		os.Exit(1)
	}
	problemService := problemapp.NewService(problems, problempg.NewRepository(db, problemfiles.NewTestdataStorage(cfg.TestdataRoot)))
	// Package builds run the same claim/lease/fence protocol as judge jobs, so
	// they get their own dispatcher and notification listener.
	buildDispatcher := authoringapp.NewDispatcher(1)
	go buildDispatcher.RunFallback(ctx, 5*time.Second)
	go listenForProblemBuilds(ctx, db, buildDispatcher)
	authoringService, err := authoringapp.NewService(
		authoringpg.NewPackageRepository(db), authoringpg.NewBuildRepository(db), authoringfiles.NewTestdataPublisher(cfg.TestdataRoot), buildDispatcher,
		cfg.BuildLeaseTTL, cfg.JudgeLongPollTimeout,
	)
	if err != nil {
		slog.Error("configure authoring service", "error", err)
		os.Exit(1)
	}
	contentService := contentapp.NewService(editorials, discussions, contentpg.NewProblemAccess(db))
	problemSetService := setapp.NewService(setpg.NewRepository(db))
	consoleService := consoleapp.NewService(consolepg.NewRepository(db))
	profileService := profileapp.NewService(profilepg.NewQueries(db))
	contestService := contestapp.NewService(contests, tokenManager)
	submissionService := submissionapp.NewService(
		submissions, problems, contestService, submissionmemory.NewSlidingWindowLimiter(time.Minute, 10), func(string) { judgeDispatcher.Notify() },
	)
	domainService := tenancyapp.NewService(tenancypg.NewRepository(db))
	router := api.Router(api.Dependencies{
		Domains:       tenancyhttp.NewHandler(domainService),
		ResolveDomain: middleware.ResolveDomain(domainService),
		PublicIDs:     publicidpg.NewResolver(db),
		Auth:          authHandler,
		Health:        api.NewHealthHandler(db.Pool.PingContext),
		Submissions:   submissionhandler.NewSubmissionHandler(submissionService),
		Problems:      problemhttp.NewProblemHandler(problemService),
		Contests: contesthttp.NewContestHandler(contestService, ratelimit.Policy{
			Limiter: abuseLimiter, Limit: cfg.ContestRegisterRateLimit, Window: cfg.RateLimitWindow,
		}),
		Editorials:     contenthttp.NewEditorialHandler(contentService),
		Discussions:    contenthttp.NewDiscussionHandler(contentService),
		AdminProblems:  problemhttp.NewAdminProblemHandler(problemService),
		AdminPackages:  authoringhandler.NewPackageHandler(authoringService),
		ProblemSets:    sethttp.NewSetHandler(problemSetService),
		Console:        consolehttp.NewConsoleHandler(consoleService),
		Builds:         authoringhandler.NewBuildHandler(authoringService, authoringhandler.DefaultBuildLimits()),
		Profiles:       profilehttp.NewProfileHandler(profileService),
		Judge:          judgehttp.NewJudgeHandler(judgeService),
		RequireAuth:    authMiddleware.Require(),
		OptionalAuth:   authMiddleware.Optional(),
		RequireAdmin:   middleware.RequireAdmin(),
		RequireJudge:   middleware.RequireJudgeService(cfg.JudgeAPIToken),
		AllowedOrigins: cfg.CORSAllowedOrigins,
		SwaggerEnabled: cfg.SwaggerEnabled,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      cfg.JudgeLongPollTimeout + 10*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		slog.Info("vertex server listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func checkHealth() error {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://127.0.0.1:" + envOr("PORT", "8080") + "/api/health/ready")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func listenForProblemBuilds(ctx context.Context, db *database.DB, dispatcher *authoringapp.Dispatcher) {
	delay := time.Second
	for ctx.Err() == nil {
		err := authoringpg.ListenBuilds(ctx, db, dispatcher.Notify)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("problem build notification listener disconnected", "error", err, "retry_in", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func listenForJudgeJobs(ctx context.Context, db *database.DB, dispatcher *judgeapp.Dispatcher) {
	delay := time.Second
	for ctx.Err() == nil {
		err := judgepg.ListenJobs(ctx, db, dispatcher.Notify)
		if ctx.Err() != nil {
			return
		}
		slog.Warn("judge job notification listener disconnected", "error", err, "retry_in", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		delay = min(delay*2, 30*time.Second)
	}
}
