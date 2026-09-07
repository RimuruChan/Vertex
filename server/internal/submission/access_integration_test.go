package submission_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/submission"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/submission/handler"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type submissionTestAuth map[string]string

func (a submissionTestAuth) Authenticate(_ context.Context, token string) (*identity.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown fixture actor")
	}
	return &identity.Identity{User: &identity.User{ID: id, Username: token, Role: "admin"}}, nil
}

var _ = Describe("Submission resource authorization against PostgreSQL", func() {
	var spaces *domain.Service
	var store *submission.SubmissionStore
	var contests *contest.ContestStore
	var users map[string]string
	var scope domain.Scope
	var task *problem.Problem
	var first, second *contest.Contest
	var practiceID, firstID, secondID string
	as := func(ctx context.Context, name string) context.Context {
		return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[name]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		spaces = domain.NewService(domain.NewStore(integrationDB))
		contests = contest.NewContestStore(integrationDB)
		store = submission.NewSubmissionStore(integrationDB)
		users = map[string]string{}
		for _, name := range []string{"manager", "setter", "problem_owner", "jury", "observer", "contestant", "reader"} {
			user, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = spaces.Create(ctx, users["manager"], domain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"setter", "problem_owner", "jury", "observer", "contestant", "reader"} {
			role := "member"
			if name == "setter" || name == "problem_owner" {
				role = "author"
			}
			Expect(spaces.SetMember(ctx, "team", users["manager"], domain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		task, err = writer.Create(as(ctx, "problem_owner"), users["problem_owner"], &problem.CreateInput{Title: "Private task", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, task.ID)).To(Succeed())
		createContest := func(title string) *contest.Contest {
			item, err := contests.Create(as(ctx, "setter"), users["setter"], &contest.PersistInput{Title: title, Rule: "icpc", Visibility: "public", Feedback: "none", BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour), RankboardVisible: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(contests.SetProblems(as(ctx, "setter"), item.ID, []contest.ProblemEntry{{ProblemID: task.ID, Label: "A"}})).To(Succeed())
			return item
		}
		first, second = createContest("First"), createContest("Second")
		seed := func(contestID *string) string {
			var id string
			Expect(integrationDB.Pool.GetContext(ctx, &id, `INSERT INTO submissions(domain_id,user_id,problem_id,contest_id,language,source_code,status,score,judged_at)
			 VALUES($1,$2,$3,$4,'cpp','private source','Accepted',100,now()) RETURNING id`, scope.Domain.ID, users["contestant"], task.ID, contestID)).To(Succeed())
			return id
		}
		practiceID, firstID, secondID = seed(nil), seed(&first.ID), seed(&second.ID)
		_, err = integrationDB.Pool.ExecContext(ctx, "INSERT INTO contest_participants(contest_id,user_id) VALUES($1,$2)", first.ID, users["contestant"])
		Expect(err).NotTo(HaveOccurred())
		Expect(contests.SetGrant(as(ctx, "setter"), first.ID, contest.GrantInput{Username: "jury", Role: contest.AccessJury})).To(Succeed())
		Expect(contests.SetGrant(as(ctx, "setter"), first.ID, contest.GrantInput{Username: "observer", Role: contest.AccessObserver})).To(Succeed())
	})

	It("limits jury mutations to the selected contest and lets observers inspect without mutating", func(ctx SpecContext) {
		_, err := store.CreateRejudging(as(ctx, "jury"), submission.RejudgeSelector{ContestID: second.ID}, users["jury"])
		Expect(err).To(MatchError(domain.ErrForbidden))
		_, err = store.CreateRejudging(as(ctx, "jury"), submission.RejudgeSelector{SubmissionIDs: []string{firstID}}, users["jury"])
		Expect(err).To(MatchError(domain.ErrForbidden))
		batch, err := store.CreateRejudging(as(ctx, "jury"), submission.RejudgeSelector{ContestID: first.ID}, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(1))
		_, err = store.Rejudging(as(ctx, "observer"), batch.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.CancelRejudging(as(ctx, "observer"), batch.ID)).To(MatchError(domain.ErrForbidden))
		_, err = store.Rejudging(as(ctx, "reader"), batch.ID)
		Expect(err).To(MatchError(domain.ErrForbidden))
		batches, err := store.ListRejudgings(as(ctx, "reader"), "", 20)
		Expect(err).NotTo(HaveOccurred())
		Expect(batches).To(BeEmpty())
		Expect(store.CancelRejudging(as(ctx, "jury"), batch.ID)).To(Succeed())
		var status string
		Expect(integrationDB.Pool.GetContext(ctx, &status, "SELECT status FROM submissions WHERE id=$1", firstID)).To(Succeed())
		Expect(status).To(Equal("Accepted"))
		Expect(integrationDB.Pool.GetContext(ctx, &status, "SELECT status FROM submissions WHERE id=$1", secondID)).To(Succeed())
		Expect(status).To(Equal("Accepted"))
	})

	It("keeps problem rejudging in practice and does not give package readers access to user source", func(ctx SpecContext) {
		batch, err := store.CreateRejudging(as(ctx, "problem_owner"), submission.RejudgeSelector{ProblemID: task.ID}, users["problem_owner"])
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(1))
		Expect(store.Rejudge(as(ctx, "problem_owner"), firstID)).To(MatchError(domain.ErrForbidden))
		Expect(store.CancelRejudging(as(ctx, "problem_owner"), batch.ID)).To(Succeed())
		writer := problem.NewProblemAdminStore(integrationDB, GinkgoT().TempDir())
		Expect(writer.SetGrant(as(ctx, "problem_owner"), task.ID, problem.GrantInput{Username: "reader", Role: problem.AccessReader})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		item, err := store.Get(as(ctx, "reader"), practiceID, submission.Viewer{UserID: users["reader"], Admin: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(item.SourceCode).To(BeEmpty())
		Expect(item.CanReadSource).To(BeFalse())
		own, err := store.Get(as(ctx, "problem_owner"), practiceID, submission.Viewer{UserID: users["problem_owner"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(own.CanReadSource).To(BeTrue())
		Expect(own.SourceCode).To(Equal("private source"))
	})

	It("uses live domain management and contest rights for hidden feedback and source", func(ctx SpecContext) {
		service := submission.NewService(store, problem.NewProblemStore(integrationDB), contest.NewService(contests, nil), nil, nil)
		for _, name := range []string{"manager", "jury", "observer"} {
			item, source, err := service.Get(as(ctx, name), firstID, users[name], "user")
			Expect(err).NotTo(HaveOccurred())
			Expect(source).To(BeTrue())
			Expect(item.Status).To(Equal("Accepted"))
			Expect(item.SourceCode).To(Equal("private source"))
		}
		item, source, err := service.Get(as(ctx, "contestant"), firstID, users["contestant"], "admin")
		Expect(err).NotTo(HaveOccurred())
		Expect(source).To(BeTrue())
		Expect(item.Status).To(Equal(submission.HiddenStatus))
		grants, err := contests.Grants(as(ctx, "setter"), first.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, grant := range grants {
			if grant.UserID != nil && *grant.UserID == users["jury"] {
				Expect(contests.RemoveGrant(as(ctx, "setter"), first.ID, grant.ID)).To(Succeed())
			}
		}
		_, err = store.Get(as(ctx, "jury"), firstID, submission.Viewer{UserID: users["jury"], Admin: true})
		Expect(err).To(MatchError(submission.ErrNotFound))
	})

	It("lets a worker finish counters while rejudge waits for its job lock", func(ctx SpecContext) {
		var jobID string
		Expect(integrationDB.Pool.GetContext(ctx, &jobID, "INSERT INTO judge_jobs(submission_id,generation,state) VALUES($1,1,'running') RETURNING id", firstID)).To(Succeed())
		worker, err := integrationDB.Pool.BeginTxx(ctx, nil)
		Expect(err).NotTo(HaveOccurred())
		defer worker.Rollback()
		var locked string
		Expect(worker.GetContext(ctx, &locked, "SELECT id FROM judge_jobs WHERE id=$1 FOR UPDATE", jobID)).To(Succeed())
		Expect(worker.GetContext(ctx, &locked, "SELECT id FROM submissions WHERE id=$1 FOR UPDATE", firstID)).To(Succeed())
		done := make(chan error, 1)
		go func() { done <- store.Rejudge(as(ctx, "jury"), firstID) }()
		Eventually(func() (int, error) {
			var waiting int
			err := integrationDB.Pool.GetContext(ctx, &waiting, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database()
			 AND wait_event_type='Lock' AND query LIKE 'UPDATE judge_jobs SET state = %'`)
			return waiting, err
		}, 3*time.Second).Should(BeNumerically(">", 0))
		finish, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, err = worker.ExecContext(finish, "UPDATE problems SET accepted_count=accepted_count+1 WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(worker.Commit()).To(Succeed())
		Eventually(done, 3*time.Second).Should(Receive(Succeed()))
		var queued int
		Expect(integrationDB.Pool.GetContext(ctx, &queued, "SELECT count(*) FROM judge_jobs WHERE submission_id=$1 AND state='queued'", firstID)).To(Succeed())
		Expect(queued).To(Equal(1))
	})

	It("serializes concurrent rejudges without leaving two queued generations", func(ctx SpecContext) {
		var start sync.WaitGroup
		start.Add(2)
		results := make(chan error, 2)
		for range 2 {
			go func() { start.Done(); start.Wait(); results <- store.Rejudge(as(ctx, "jury"), firstID) }()
		}
		Eventually(results, 5*time.Second).Should(Receive(Succeed()))
		Eventually(results, 5*time.Second).Should(Receive(Succeed()))
		var generation, queued int
		Expect(integrationDB.Pool.GetContext(ctx, &generation, "SELECT judge_generation FROM submissions WHERE id=$1", firstID)).To(Succeed())
		Expect(generation).To(Equal(3))
		Expect(integrationDB.Pool.GetContext(ctx, &queued, "SELECT count(*) FROM judge_jobs WHERE submission_id=$1 AND state='queued'", firstID)).To(Succeed())
		Expect(queued).To(Equal(1))
	})

	It("exposes scoped rejudging to jury without trusting an administrator claim", func(ctx SpecContext) {
		service := submission.NewService(store, problem.NewProblemStore(integrationDB), contest.NewService(contests, nil), nil, nil)
		handler := submissionhandler.NewSubmissionHandler(service)
		auth := middleware.NewAuthMiddleware(submissionTestAuth(users))
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
		request := func(method, path, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/team"+path, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		Expect(request("POST", "/admin/rejudgings", "reader", `{"contestId":"`+first.ID+`"}`).Code).To(Equal(403))
		Expect(request("POST", "/admin/rejudgings", "jury", `{"contestId":"`+second.ID+`"}`).Code).To(Equal(403))
		response := request("POST", "/admin/rejudgings", "jury", `{"contestId":"`+first.ID+`"}`)
		Expect(response.Code).To(Equal(202))
		var batch struct {
			ID string `json:"id"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &batch)).To(Succeed())
		Expect(batch.ID).NotTo(BeEmpty())
		Expect(request("GET", "/admin/rejudgings/"+batch.ID, "observer", "").Code).To(Equal(200))
		Expect(request("POST", "/admin/rejudgings/"+batch.ID+"/cancel", "observer", "").Code).To(Equal(403))
		Expect(request("POST", "/admin/rejudgings/"+batch.ID+"/cancel", "jury", "").Code).To(Equal(200))
		Expect(request("GET", "/submissions/"+firstID, "jury", "").Body.String()).To(ContainSubstring(`"sourceCode":"private source"`))
		Expect(request("GET", "/submissions/"+firstID, "contestant", "").Body.String()).To(ContainSubstring(`"status":"Submitted"`))
		Expect(request("GET", "/admin/rejudgings", "", "").Code).To(Equal(401))
	})
})
