package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/modules/contest/application"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	contestpg "github.com/RimuruChan/Vertex/server/internal/modules/contest/infrastructure/postgres"
	dto "github.com/RimuruChan/Vertex/server/internal/modules/contest/transport/http/dto"
	identityapp "github.com/RimuruChan/Vertex/server/internal/modules/identity/application"
	identitydomain "github.com/RimuruChan/Vertex/server/internal/modules/identity/domain"
	identitypg "github.com/RimuruChan/Vertex/server/internal/modules/identity/infrastructure/postgres"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/modules/problem/domain"
	problemfiles "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/filesystem"
	problempg "github.com/RimuruChan/Vertex/server/internal/modules/problem/infrastructure/postgres"
	publicidpg "github.com/RimuruChan/Vertex/server/internal/modules/publicid/infrastructure/postgres"
	submissionapp "github.com/RimuruChan/Vertex/server/internal/modules/submission/application"
	submissiondomain "github.com/RimuruChan/Vertex/server/internal/modules/submission/domain"
	submission "github.com/RimuruChan/Vertex/server/internal/modules/submission/infrastructure/postgres"
	submissionhandler "github.com/RimuruChan/Vertex/server/internal/modules/submission/transport/http"
	tenancyapp "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/application"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	tenancypg "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/infrastructure/postgres"
	"github.com/RimuruChan/Vertex/server/internal/platform/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/transport/http"
	"github.com/RimuruChan/Vertex/server/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type submissionTestAuth map[string]string

func (a submissionTestAuth) Authenticate(_ context.Context, token string) (*identityapp.Identity, error) {
	id, ok := a[token]
	if !ok {
		return nil, errors.New("unknown fixture actor")
	}
	return &identityapp.Identity{User: &identitydomain.User{ID: id, Username: token, Role: "admin"}}, nil
}

var _ = Describe("Submission resource authorization against PostgreSQL", func() {
	var spaces *tenancyapp.Service
	var store *submission.Repository
	var contests *contestpg.Repository
	var users map[string]string
	var scope tenancydomain.Scope
	var task *problemdomain.Problem
	var first, second *contestdomain.Contest
	var practiceID, firstID, secondID string
	as := func(ctx context.Context, name string) context.Context {
		return tenancydomain.WithScope(ctx, tenancydomain.Scope{Domain: scope.Domain, UserID: users[name]})
	}
	BeforeEach(func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		spaces = tenancyapp.NewService(tenancypg.NewRepository(integrationDB))
		contests = contestpg.NewRepository(integrationDB)
		store = submission.NewRepository(integrationDB)
		users = map[string]string{}
		for _, name := range []string{"manager", "setter", "problem_owner", "jury", "observer", "contestant", "reader"} {
			user, err := identitypg.NewUserRepository(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = user.ID
		}
		var err error
		scope, err = spaces.Create(ctx, users["manager"], tenancydomain.CreateInput{Slug: "team", Name: "Team"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"setter", "problem_owner", "jury", "observer", "contestant", "reader"} {
			role := "member"
			if name == "setter" || name == "problem_owner" {
				role = "author"
			}
			Expect(spaces.SetMember(ctx, "team", users["manager"], tenancydomain.MemberInput{Username: name, RoleKey: role, Status: "active"})).To(Succeed())
		}
		writer := problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir()))
		task, err = writer.Create(as(ctx, "problem_owner"), users["problem_owner"], &problemdomain.CreateInput{Title: "Private task", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(dbtest.PublishedProblems(ctx, integrationDB, task.ID)).To(Succeed())
		createContest := func(title string) *contestdomain.Contest {
			item, err := contests.Create(as(ctx, "setter"), users["setter"], &contestdomain.PersistInput{Title: title, Rule: "icpc", Visibility: "public", Feedback: "none", BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour), RankboardVisible: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(contests.SetProblems(as(ctx, "setter"), item.ID, []contestdomain.ProblemEntry{{ProblemID: task.ID, Label: "A"}})).To(Succeed())
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
		Expect(contests.SetGrant(as(ctx, "setter"), first.ID, contestdomain.GrantInput{Username: "jury", Role: contestdomain.AccessJury})).To(Succeed())
		Expect(contests.SetGrant(as(ctx, "setter"), first.ID, contestdomain.GrantInput{Username: "observer", Role: contestdomain.AccessObserver})).To(Succeed())
	})

	// Exercise the real policy, SQL projections, DTOs and router. Raw JSON maps
	// deliberately catch fields a narrow test response struct would discard.
	readJSON := func(actor, path string, status int) (map[string]any, string) {
		gin.SetMode(gin.TestMode)
		service := submissionapp.NewService(store, problempg.NewQueries(integrationDB), contestapp.NewService(contests, nil), nil, nil)
		handler := submissionhandler.NewSubmissionHandler(service)
		auth := middleware.NewAuthMiddleware(submissionTestAuth(users))
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicidpg.NewResolver(integrationDB)))
		r := httptest.NewRequest(http.MethodGet, "/api/domains/team"+path, nil)
		if actor != "" {
			r.Header.Set("Authorization", "Bearer "+actor)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, r)
		Expect(response.Code).To(Equal(status), response.Body.String())
		var result map[string]any
		Expect(json.Unmarshal(response.Body.Bytes(), &result)).To(Succeed())
		return result, response.Body.String()
	}
	seedPrivateResult := func(ctx context.Context) {
		_, err := integrationDB.Pool.ExecContext(ctx, `UPDATE submissions SET status='Wrong Answer',score=40,source_code='PRIVATE_SOURCE_SENTINEL',compile_result='PRIVATE_COMPILER_SENTINEL PRIVATE_SOURCE_SENTINEL',total_time_ms=333,peak_memory_kb=777,judged_cases=3,total_cases=10,judged_at=now(),case_results=$2::jsonb WHERE id=$1`, firstID,
			`[{"caseIndex":1,"verdict":"Accepted","timeMs":7,"memoryKb":1234,"checkerOutput":"PRIVATE_CHECKER_SENTINEL","exitStatus":"PRIVATE_EXIT_SENTINEL"},{"caseIndex":4,"verdict":"Wrong Answer","timeMs":19,"memoryKb":4321,"checkerOutput":"PRIVATE_CHECKER_SENTINEL"},{"caseIndex":2,"verdict":"Time Limit Exceeded","timeMs":99,"memoryKb":4567,"checkerOutput":"PRIVATE_CHECKER_SENTINEL","exitStatus":"PRIVATE_EXIT_SENTINEL"}]`)
		Expect(err).NotTo(HaveOccurred())
	}
	assertPrivateFieldsAbsent := func(body map[string]any, casesHidden bool) {
		for _, field := range []string{"compileResult", "judgedAt"} {
			Expect(body).NotTo(HaveKey(field))
		}
		for _, field := range []string{"totalTimeMs", "peakMemoryKb", "judgedCases", "totalCases"} {
			Expect(body).To(HaveKeyWithValue(field, BeNumerically("==", 0)))
		}
		if casesHidden {
			Expect(body).NotTo(HaveKey("caseResults"))
		}
	}

	It("never serializes withheld feedback through detail, progress or list", func(ctx SpecContext) {
		seedPrivateResult(ctx)
		for _, level := range []string{"none", "summary", "first_error", "full"} {
			_, err := integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET feedback=$2,submission_visibility='during' WHERE id=$1`, first.ID, level)
			Expect(err).NotTo(HaveOccurred())
			for _, actor := range []string{"contestant", "reader", "jury", "observer"} {
				staff := actor == "jury" || actor == "observer"
				for _, endpoint := range []string{"detail", "progress", "list"} {
					path := "/submissions/" + firstID
					if endpoint == "progress" {
						path += "/progress"
					}
					if endpoint == "list" {
						path = "/submissions?contest=" + first.PublicID
					}
					body, raw := readJSON(actor, path, 200)
					if endpoint == "list" {
						Expect(body["total"]).To(BeNumerically("==", 1))
						items := body["items"].([]any)
						Expect(items).To(HaveLen(1))
						body = items[0].(map[string]any)
					}
					if endpoint != "detail" || actor == "reader" {
						Expect(body).NotTo(HaveKey("sourceCode"))
					} else {
						Expect(body["sourceCode"]).To(Equal("PRIVATE_SOURCE_SENTINEL"))
					}
					if endpoint == "progress" {
						for _, field := range []string{"userId", "username", "problemId", "problemTitle", "contestId", "submittedAt", "judgedAt", "problemVersion"} {
							Expect(body).NotTo(HaveKey(field))
						}
					}
					if !staff && level != "full" {
						assertPrivateFieldsAbsent(body, level != "first_error" || endpoint == "list")
						for _, marker := range []string{"PRIVATE_COMPILER_SENTINEL", "PRIVATE_CHECKER_SENTINEL", "PRIVATE_EXIT_SENTINEL"} {
							Expect(raw).NotTo(ContainSubstring(marker))
						}
						if level == "none" {
							Expect(body["status"]).To(Equal("Submitted"))
							Expect(body["score"]).To(BeNumerically("==", 0))
						} else {
							Expect(body["status"]).To(Equal("Wrong Answer"))
						}
						if level == "first_error" && endpoint != "list" {
							Expect(body["caseResults"]).To(Equal([]any{map[string]any{"caseIndex": float64(2), "verdict": "Time Limit Exceeded", "timeMs": float64(0), "memoryKb": float64(0)}}))
						}
					} else {
						Expect(body["status"]).To(Equal("Wrong Answer"))
						Expect(body["score"]).To(BeNumerically("==", 40))
						Expect(body["totalTimeMs"]).To(BeNumerically("==", 333))
						if endpoint != "list" {
							Expect(body["caseResults"]).To(HaveLen(3))
							Expect(raw).To(ContainSubstring("PRIVATE_CHECKER_SENTINEL"))
						}
					}
				}
			}
		}
	})

	It("keeps frozen results, status filters and hidden-row counts private on the wire", func(ctx SpecContext) {
		seedPrivateResult(ctx)
		_, err := integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET feedback='full',submission_visibility='during',source_code_visibility='after_end',freeze_at=now()-interval '30 minutes',frozen_submission_visibility='pending' WHERE id=$1`, first.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, suffix := range []string{"", "/progress"} {
			body, raw := readJSON("reader", "/submissions/"+firstID+suffix, 200)
			Expect(body["status"]).To(Equal("Pending"))
			Expect(body["score"]).To(BeNumerically("==", 0))
			assertPrivateFieldsAbsent(body, true)
			Expect(raw).NotTo(ContainSubstring("PRIVATE_"))
		}
		for _, status := range []string{"Accepted", "Wrong Answer", "Submitted", "Pending"} {
			body, raw := readJSON("reader", "/submissions?contest="+first.PublicID+"&status="+url.QueryEscape(status), 200)
			if status == "Pending" {
				Expect(body["total"]).To(BeNumerically("==", 1))
				item := body["items"].([]any)[0].(map[string]any)
				assertPrivateFieldsAbsent(item, true)
			} else {
				Expect(body["total"]).To(BeNumerically("==", 0))
				Expect(body["items"]).To(BeEmpty())
			}
			Expect(raw).NotTo(ContainSubstring("PRIVATE_"))
		}
		_, err = integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET frozen_submission_visibility='hidden' WHERE id=$1`, first.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, suffix := range []string{"", "/progress"} {
			body, raw := readJSON("reader", "/submissions/"+firstID+suffix, 404)
			Expect(body).To(HaveLen(2))
			Expect(raw).NotTo(ContainSubstring("PRIVATE_"))
		}
		body, _ := readJSON("reader", "/submissions?contest="+first.PublicID, 200)
		Expect(body["total"]).To(BeNumerically("==", 0))
		Expect(body["items"]).To(BeEmpty())
		_, err = integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET end_at=now()-interval '2 seconds',unfreeze_at=now()-interval '1 second' WHERE id=$1`, first.ID)
		Expect(err).NotTo(HaveOccurred())
		body, _ = readJSON("reader", "/submissions/"+firstID, 200)
		Expect(body["sourceCode"]).To(Equal("PRIVATE_SOURCE_SENTINEL"))
		Expect(body["score"]).To(BeNumerically("==", 40))
		Expect(body["caseResults"]).To(HaveLen(3))
	})

	It("does not leak private source through compiler diagnostics on detail or progress", func(ctx SpecContext) {
		seedPrivateResult(ctx)
		_, err := integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET feedback='full',submission_visibility='during',source_code_visibility='own' WHERE id=$1`, first.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx, `UPDATE submissions SET status='Compile Error',case_results='[]'::jsonb WHERE id=$1`, firstID)
		Expect(err).NotTo(HaveOccurred())
		for _, suffix := range []string{"", "/progress"} {
			body, raw := readJSON("reader", "/submissions/"+firstID+suffix, 200)
			Expect(body).NotTo(HaveKey("compileResult"))
			Expect(raw).NotTo(ContainSubstring("PRIVATE_SOURCE_SENTINEL"))
			for _, actor := range []string{"contestant", "jury", "observer"} {
				body, _ = readJSON(actor, "/submissions/"+firstID+suffix, 200)
				Expect(body["compileResult"]).To(ContainSubstring("PRIVATE_SOURCE_SENTINEL"))
			}
		}
	})
	It("drops privileged wire fields immediately after a jury grant is revoked", func(ctx SpecContext) {
		seedPrivateResult(ctx)
		_, err := integrationDB.Pool.ExecContext(ctx, `UPDATE contests SET feedback='none',submission_visibility='during' WHERE id=$1`, first.ID)
		Expect(err).NotTo(HaveOccurred())
		body, _ := readJSON("jury", "/submissions/"+firstID, 200)
		Expect(body["compileResult"]).To(ContainSubstring("PRIVATE_COMPILER_SENTINEL"))
		grants, err := contests.Grants(as(ctx, "setter"), first.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, grant := range grants {
			if grant.Role == contestdomain.AccessJury && grant.UserID != nil && *grant.UserID == users["jury"] {
				Expect(contests.RemoveGrant(as(ctx, "setter"), first.ID, grant.ID)).To(Succeed())
			}
		}
		for _, suffix := range []string{"", "/progress"} {
			body, raw := readJSON("jury", "/submissions/"+firstID+suffix, 200)
			Expect(body["status"]).To(Equal("Submitted"))
			assertPrivateFieldsAbsent(body, true)
			Expect(raw).NotTo(ContainSubstring("PRIVATE_"))
		}
	})

	It("limits jury mutations to the selected contest and lets observers inspect without mutating", func(ctx SpecContext) {
		_, err := store.CreateRejudging(as(ctx, "jury"), submissiondomain.RejudgeSelector{ContestID: second.ID}, users["jury"])
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		_, err = store.CreateRejudging(as(ctx, "jury"), submissiondomain.RejudgeSelector{SubmissionIDs: []string{firstID}}, users["jury"])
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
		batch, err := store.CreateRejudging(as(ctx, "jury"), submissiondomain.RejudgeSelector{ContestID: first.ID}, users["jury"])
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(1))
		_, err = store.Rejudging(as(ctx, "observer"), batch.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(store.CancelRejudging(as(ctx, "observer"), batch.ID)).To(MatchError(tenancydomain.ErrForbidden))
		_, err = store.Rejudging(as(ctx, "reader"), batch.ID)
		Expect(err).To(MatchError(tenancydomain.ErrForbidden))
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

	It("filters hidden feedback by visible status so counts cannot reveal the raw verdict", func(ctx SpecContext) {
		service := submissionapp.NewService(store, nil, contestapp.NewService(contests, nil), nil, nil)
		list := func(name, status string, contestID string) ([]submissiondomain.Submission, int) {
			items, total, err := service.List(as(ctx, name), submissiondomain.Filters{ContestID: contestID, UserID: users["contestant"], Status: status, Limit: 20}, users[name], "admin")
			Expect(err).NotTo(HaveOccurred())
			return items, total
		}
		items, total := list("contestant", "Accepted", first.ID)
		Expect(total).To(BeZero())
		Expect(items).To(BeEmpty())
		items, total = list("contestant", "Submitted", first.ID)
		Expect(total).To(Equal(1))
		Expect(items[0].Status).To(Equal("Submitted"))
		Expect(items[0].Score).To(BeZero())
		_, total = list("contestant", "Accepted", "")
		Expect(total).To(Equal(1)) // public practice only
		_, total = list("contestant", "Submitted", "")
		Expect(total).To(Equal(2))
		for _, name := range []string{"jury", "observer", "setter", "manager"} {
			items, total = list(name, "Accepted", first.ID)
			Expect(total).To(Equal(1))
			Expect(items[0].Status).To(Equal("Accepted"))
		}
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET feedback='summary' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		items, total = list("contestant", "Accepted", first.ID)
		Expect(total).To(Equal(1))
		Expect(items[0].TotalCases).To(BeZero())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET feedback='none',end_at=now()-interval '1 second' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		items, total = list("contestant", "Accepted", first.ID)
		Expect(total).To(Equal(1))
		Expect(items[0].Status).To(Equal("Accepted"))
	})

	It("enforces configurable peer records, frozen projections and post-contest source access", func(ctx SpecContext) {
		reader := submissiondomain.Viewer{UserID: users["reader"]}
		get := func() (*submissiondomain.Submission, error) { return store.Get(as(ctx, "reader"), firstID, reader) }
		_, err := get()
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET submission_visibility='during', source_code_visibility='after_end', feedback='full' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		record, err := get()
		Expect(err).NotTo(HaveOccurred())
		Expect(record.Status).To(Equal("Accepted"))
		Expect(record.SourceCode).To(BeEmpty()) // Never share source while the contest is running.
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET freeze_at=now()-interval '1 minute',frozen_submission_visibility='pending' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		record, err = get()
		Expect(err).NotTo(HaveOccurred())
		Expect(record.Status).To(Equal("Pending"))
		Expect(record.Score).To(BeZero())
		Expect(record.JudgedAt).To(BeNil())
		progress, err := store.Progress(as(ctx, "reader"), firstID, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(progress.Status).To(Equal("Pending"))
		Expect(progress.Score).To(BeZero())
		items, total, err := store.List(as(ctx, "reader"), submissiondomain.Filters{ContestID: first.ID, Status: "Pending", Limit: 10}, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items[0].Status).To(Equal("Pending"))
		_, total, err = store.List(as(ctx, "reader"), submissiondomain.Filters{ContestID: first.ID, Status: "Accepted", Limit: 10}, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		own, err := store.Get(as(ctx, "contestant"), firstID, submissiondomain.Viewer{UserID: users["contestant"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(own.Status).To(Equal("Accepted"))
		for _, staff := range []string{"jury", "observer"} {
			record, err := store.Get(as(ctx, staff), firstID, submissiondomain.Viewer{UserID: users[staff]})
			Expect(err).NotTo(HaveOccurred())
			Expect(record.Status).To(Equal("Accepted"))
			Expect(record.SourceCode).To(Equal("private source"))
		}
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET frozen_submission_visibility='hidden' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = get()
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
		_, total, err = store.List(as(ctx, "reader"), submissiondomain.Filters{ContestID: first.ID, Limit: 10}, reader)
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(BeZero())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET end_at=now()-interval '2 seconds',unfreeze_at=now()-interval '1 second' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		record, err = get()
		Expect(err).NotTo(HaveOccurred())
		Expect(record.SourceCode).To(Equal("private source"))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET source_code_visibility='own' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		record, err = get()
		Expect(err).NotTo(HaveOccurred())
		Expect(record.SourceCode).To(BeEmpty())
	})

	It("reveals complete public standings after unfreeze without claiming jury privileges", func(ctx SpecContext) {
		_, err := integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET feedback='full', freeze_at=now()-interval '1 minute' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = integrationDB.Pool.ExecContext(ctx, `INSERT INTO contest_submission_cells(domain_id,contest_id,user_id,problem_id,attempts,penalty_sec,score,solved_at,pending_count) VALUES($1,$2,$3,$4,1,120,100,now(),1)`, scope.Domain.ID, first.ID, users["contestant"], task.ID)
		Expect(err).NotTo(HaveOccurred())
		service := contestapp.NewService(contests, nil)
		before, err := service.Rankboard(as(ctx, "contestant"), first.ID, users["contestant"], "admin", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(before.Frozen).To(BeTrue())
		Expect(before.JuryView).To(BeFalse())
		visible := dto.FromRankboard(before)
		Expect(visible.Rows[0].Solved).To(BeZero())
		Expect(visible.Rows[0].Cells[0].Score).To(BeZero())
		Expect(visible.Rows[0].Cells[0].PendingCount).To(Equal(1))
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET unfreeze_at=now()-interval '1 second' WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		after, err := service.Rankboard(as(ctx, "contestant"), first.ID, users["contestant"], "user", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(after.Frozen).To(BeFalse())
		Expect(after.JuryView).To(BeFalse())
		visible = dto.FromRankboard(after)
		Expect(visible.Rows[0].Solved).To(Equal(1))
		Expect(visible.Rows[0].Cells[0].Score).To(Equal(100))
		Expect(visible.Rows[0].Cells[0].FirstSolver).To(BeTrue())
		Expect(visible.Rows[0].Cells[0].PendingCount).To(BeZero())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE contests SET begin_at=now()+interval '1 hour',end_at=now()+interval '2 hour',freeze_at=NULL,unfreeze_at=NULL WHERE id=$1", first.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = service.Rankboard(as(ctx, "contestant"), first.ID, users["contestant"], "admin", true)
		Expect(err).To(MatchError(contestdomain.ErrNotFound))
	})

	It("keeps problem rejudging in practice and does not give package readers access to user source", func(ctx SpecContext) {
		batch, err := store.CreateRejudging(as(ctx, "problem_owner"), submissiondomain.RejudgeSelector{ProblemID: task.ID}, users["problem_owner"])
		Expect(err).NotTo(HaveOccurred())
		Expect(batch.TotalCount).To(Equal(1))
		Expect(store.Rejudge(as(ctx, "problem_owner"), firstID)).To(MatchError(tenancydomain.ErrForbidden))
		Expect(store.CancelRejudging(as(ctx, "problem_owner"), batch.ID)).To(Succeed())
		writer := problempg.NewRepository(integrationDB, problemfiles.NewTestdataStorage(GinkgoT().TempDir()))
		Expect(writer.SetGrant(as(ctx, "problem_owner"), task.ID, problemdomain.GrantInput{Username: "reader", Role: problemdomain.AccessReader})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE problems SET visibility='private' WHERE id=$1", task.ID)
		Expect(err).NotTo(HaveOccurred())
		item, err := store.Get(as(ctx, "reader"), practiceID, submissiondomain.Viewer{UserID: users["reader"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(item.SourceCode).To(BeEmpty())
		Expect(item.CanReadSource).To(BeFalse())
		own, err := store.Get(as(ctx, "problem_owner"), practiceID, submissiondomain.Viewer{UserID: users["problem_owner"]})
		Expect(err).NotTo(HaveOccurred())
		Expect(own.CanReadSource).To(BeTrue())
		Expect(own.SourceCode).To(Equal("private source"))
	})

	It("uses live domain management and contest rights for hidden feedback and source", func(ctx SpecContext) {
		service := submissionapp.NewService(store, problempg.NewQueries(integrationDB), contestapp.NewService(contests, nil), nil, nil)
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
		Expect(item.Status).To(Equal(submissiondomain.HiddenStatus))
		grants, err := contests.Grants(as(ctx, "setter"), first.ID)
		Expect(err).NotTo(HaveOccurred())
		for _, grant := range grants {
			if grant.UserID != nil && *grant.UserID == users["jury"] {
				Expect(contests.RemoveGrant(as(ctx, "setter"), first.ID, grant.ID)).To(Succeed())
			}
		}
		_, err = store.Get(as(ctx, "jury"), firstID, submissiondomain.Viewer{UserID: users["jury"]})
		Expect(err).To(MatchError(submissiondomain.ErrNotFound))
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
			 AND wait_event_type='Lock' AND query LIKE '%UPDATE judge_jobs SET state = %'`)
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
		service := submissionapp.NewService(store, problempg.NewQueries(integrationDB), contestapp.NewService(contests, nil), nil, nil)
		handler := submissionhandler.NewSubmissionHandler(service)
		auth := middleware.NewAuthMiddleware(submissionTestAuth(users))
		router := gin.New()
		handler.RegisterRoutes(router.Group("/api/domains/:domain"), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicidpg.NewResolver(integrationDB)))
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
		Expect(request("GET", "/submissions?contest="+first.PublicID+"&status=Accepted", "contestant", "").Body.String()).To(ContainSubstring(`"total":0`))
		Expect(request("GET", "/submissions?contest="+first.PublicID+"&status=Submitted", "contestant", "").Body.String()).To(ContainSubstring(`"total":1`))
		Expect(request("GET", "/admin/rejudgings", "", "").Code).To(Equal(401))
	})
})
