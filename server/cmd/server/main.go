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
	"github.com/RimuruChan/Vertex/server/internal/authoring"
	authoringhandler "github.com/RimuruChan/Vertex/server/internal/authoring/handler"
	"github.com/RimuruChan/Vertex/server/internal/config"
	"github.com/RimuruChan/Vertex/server/internal/console"
	consolehandler "github.com/RimuruChan/Vertex/server/internal/console/handler"
	"github.com/RimuruChan/Vertex/server/internal/content"
	contenthandler "github.com/RimuruChan/Vertex/server/internal/content/handler"
	"github.com/RimuruChan/Vertex/server/internal/contest"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	"github.com/RimuruChan/Vertex/server/internal/database"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	domainhandler "github.com/RimuruChan/Vertex/server/internal/domain/handler"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	identityhandler "github.com/RimuruChan/Vertex/server/internal/identity/handler"
	"github.com/RimuruChan/Vertex/server/internal/judge"
	judgehandler "github.com/RimuruChan/Vertex/server/internal/judge/handler"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	"github.com/RimuruChan/Vertex/server/internal/problemset"
	problemsethandler "github.com/RimuruChan/Vertex/server/internal/problemset/handler"
	"github.com/RimuruChan/Vertex/server/internal/profile"
	profilehandler "github.com/RimuruChan/Vertex/server/internal/profile/handler"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/handler"
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

	if err := identity.BootstrapAdmin(
		ctx, db, cfg.AdminUsername, cfg.AdminPassword, cfg.AdminEmail, identity.HashPassword,
	); err != nil {
		slog.Error("bootstrap administrator", "error", err)
		os.Exit(1)
	}

	users := identity.NewUserStore(db)
	submissions := submission.NewSubmissionStore(db)
	problems := problem.NewProblemStore(db)
	contests := contest.NewContestStore(db)
	editorials := content.NewEditorialStore(db)
	discussions := content.NewDiscussionStore(db)
	tokenManager, err := identity.NewManager(cfg.JWTSecret, cfg.AccessTokenTTL)
	if err != nil {
		slog.Error("configure token manager", "error", err)
		os.Exit(1)
	}
	authService, err := identity.NewService(users, identity.NewSessionStore(db), tokenManager, cfg.RefreshTokenTTL)
	if err != nil {
		slog.Error("configure authentication", "error", err)
		os.Exit(1)
	}
	abuseLimiter := ratelimit.New(cfg.RateLimitMaxKeys)
	authHandler := identityhandler.NewAuthHandler(authService, identityhandler.AuthCookieConfig{
		Secure: cfg.AuthCookieSecure, Domain: cfg.AuthCookieDomain, Lifetime: cfg.RefreshTokenTTL,
	}, identityhandler.AuthRateLimits{
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
	judgeDispatcher := judge.NewDispatcher(1)
	go judgeDispatcher.RunFallback(ctx, 5*time.Second)
	go listenForJudgeJobs(ctx, db, judgeDispatcher)
	judgeService, err := judge.NewService(judge.NewJudgeJobStore(db), judgeDispatcher, cfg.JudgeLeaseTTL, cfg.JudgeLongPollTimeout)
	if err != nil {
		slog.Error("configure judge service", "error", err)
		os.Exit(1)
	}
	problemService := problem.NewService(problems, problem.NewProblemAdminStore(db, cfg.TestdataRoot))
	// Package builds run the same claim/lease/fence protocol as judge jobs, so
	// they get their own dispatcher and notification listener.
	buildDispatcher := authoring.NewDispatcher(1)
	go buildDispatcher.RunFallback(ctx, 5*time.Second)
	go listenForProblemBuilds(ctx, db, buildDispatcher)
	authoringService, err := authoring.NewService(
		authoring.NewPackageStore(db), authoring.NewBuildStore(db),
		authoring.NewTestdataPublisher(cfg.TestdataRoot), buildDispatcher,
		cfg.BuildLeaseTTL, cfg.JudgeLongPollTimeout,
	)
	if err != nil {
		slog.Error("configure authoring service", "error", err)
		os.Exit(1)
	}
	contentService := content.NewService(editorials, discussions, content.NewAccessStore(db))
	problemSetService := problemset.NewService(problemset.NewSetStore(db))
	consoleService := console.NewService(console.NewConsoleStore(db))
	profileService := profile.NewService(profile.NewProfileStore(db))
	contestService := contest.NewService(contests, tokenManager)
	submissionService := submission.NewService(
		submissions, problems, contestService,
		submission.NewSlidingWindowLimiter(time.Minute, 10),
		func(string) { judgeDispatcher.Notify() },
	)
	router := api.Router(api.Dependencies{
		Domains:     domainhandler.NewHandler(domain.NewService(domain.NewStore(db))),
		PublicIDs:   publicid.NewStore(db),
		Auth:        authHandler,
		Health:      api.NewHealthHandler(db.Pool.PingContext),
		Submissions: submissionhandler.NewSubmissionHandler(submissionService),
		Problems:    problemhandler.NewProblemHandler(problemService),
		Contests: contesthandler.NewContestHandler(contestService, ratelimit.Policy{
			Limiter: abuseLimiter, Limit: cfg.ContestRegisterRateLimit, Window: cfg.RateLimitWindow,
		}),
		Editorials:     contenthandler.NewEditorialHandler(contentService),
		Discussions:    contenthandler.NewDiscussionHandler(contentService),
		AdminProblems:  problemhandler.NewAdminProblemHandler(problemService),
		AdminPackages:  authoringhandler.NewPackageHandler(authoringService),
		ProblemSets:    problemsethandler.NewSetHandler(problemSetService),
		Console:        consolehandler.NewConsoleHandler(consoleService),
		Builds:         authoringhandler.NewBuildHandler(authoringService, authoringhandler.DefaultBuildLimits()),
		Profiles:       profilehandler.NewProfileHandler(profileService),
		Judge:          judgehandler.NewJudgeHandler(judgeService),
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

func listenForProblemBuilds(ctx context.Context, db *database.DB, dispatcher *authoring.Dispatcher) {
	delay := time.Second
	for ctx.Err() == nil {
		err := authoring.ListenBuilds(ctx, db, dispatcher.Notify)
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

func listenForJudgeJobs(ctx context.Context, db *database.DB, dispatcher *judge.Dispatcher) {
	delay := time.Second
	for ctx.Err() == nil {
		err := judge.ListenJobs(ctx, db, dispatcher.Notify)
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
