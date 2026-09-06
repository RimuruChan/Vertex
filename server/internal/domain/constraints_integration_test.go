package domain_test

import (
	"errors"
	"fmt"

	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Resource relationship constraints", func() {
	It("rejects cross-domain foreign keys independently of the services", func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		user, err := identity.NewUserStore(integrationDB).Create(ctx, "owner", "owner@example.test", "fixture-hash")
		Expect(err).NotTo(HaveOccurred())
		scope, err := domain.NewService(domain.NewStore(integrationDB)).Create(ctx, user.ID, domain.CreateInput{Slug: "training", Name: "Training"})
		Expect(err).NotTo(HaveOccurred())
		type fixture struct {
			domain, problem, contest, set, editorial, submission, rejudging string
			tag, post, clarification                                        int64
		}
		seed := func(domainID string) fixture {
			item := fixture{domain: domainID}
			Expect(integrationDB.Pool.GetContext(ctx, &item.problem, "INSERT INTO problems(domain_id,title,owner_id) VALUES($1,'Problem',$2) RETURNING id", domainID, user.ID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.contest, "INSERT INTO contests(domain_id,title,begin_at,end_at) VALUES($1,'Contest',now(),now()) RETURNING id", domainID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.set, "INSERT INTO problem_sets(domain_id,title) VALUES($1,'Set') RETURNING id", domainID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.editorial, "INSERT INTO editorials(domain_id,problem_id,title) VALUES($1,$2,'Editorial') RETURNING id", domainID, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.submission, "INSERT INTO submissions(domain_id,user_id,problem_id,language,source_code) VALUES($1,$2,$3,'cpp','source') RETURNING id", domainID, user.ID, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.rejudging, "INSERT INTO rejudgings(domain_id) VALUES($1) RETURNING id", domainID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.tag, "INSERT INTO tags(domain_id,name) VALUES($1,'Tag') RETURNING id", domainID)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.post, "INSERT INTO discussion_posts(domain_id,problem_id,content_md) VALUES($1,$2,'Post') RETURNING id", domainID, item.problem)).To(Succeed())
			Expect(integrationDB.Pool.GetContext(ctx, &item.clarification, "INSERT INTO clarifications(domain_id,contest_id,body,from_jury) VALUES($1,$2,'Announcement',true) RETURNING id", domainID, item.contest)).To(Succeed())
			return item
		}
		a, b := seed(domain.OfficialID), seed(scope.Domain.ID)
		cases := []struct {
			name, query string
			args        []any
		}{
			{"submission problem", "UPDATE submissions SET problem_id=$2 WHERE id=$1", []any{a.submission, b.problem}},
			{"submission contest", "UPDATE submissions SET contest_id=$2 WHERE id=$1", []any{a.submission, b.contest}},
			{"editorial problem", "UPDATE editorials SET problem_id=$2 WHERE id=$1", []any{a.editorial, b.problem}},
			{"discussion problem", "UPDATE discussion_posts SET problem_id=$2 WHERE id=$1", []any{a.post, b.problem}},
			{"discussion editorial", "UPDATE discussion_posts SET problem_id=NULL,editorial_id=$2 WHERE id=$1", []any{a.post, b.editorial}},
			{"discussion contest", "UPDATE discussion_posts SET problem_id=NULL,contest_id=$2 WHERE id=$1", []any{a.post, b.contest}},
			{"discussion parent", "UPDATE discussion_posts SET parent_id=$2 WHERE id=$1", []any{a.post, b.post}},
			{"rejudging contest", "UPDATE rejudgings SET contest_id=$2 WHERE id=$1", []any{a.rejudging, b.contest}},
			{"rejudging problem", "UPDATE rejudgings SET problem_id=$2 WHERE id=$1", []any{a.rejudging, b.problem}},
			{"problem tag", "INSERT INTO problem_tags(problem_id,tag_id) VALUES($1,$2)", []any{a.problem, b.tag}},
			{"tag problem", "INSERT INTO problem_tags(problem_id,tag_id) VALUES($1,$2)", []any{b.problem, a.tag}},
			{"contest problem", "INSERT INTO contest_problems(contest_id,problem_id) VALUES($1,$2)", []any{a.contest, b.problem}},
			{"problem contest", "INSERT INTO contest_problems(contest_id,problem_id) VALUES($1,$2)", []any{b.contest, a.problem}},
			{"set problem", "INSERT INTO problem_set_problems(set_id,problem_id) VALUES($1,$2)", []any{a.set, b.problem}},
			{"problem set", "INSERT INTO problem_set_problems(set_id,problem_id) VALUES($1,$2)", []any{b.set, a.problem}},
			{"rejudging submission", "INSERT INTO rejudging_submissions(rejudging_id,submission_id,generation,prior_status,prior_score,prior_total_time_ms,prior_peak_memory_kb,prior_compile_result,prior_case_results,prior_judged_cases,prior_total_cases) VALUES($1,$2,1,'Accepted',100,0,0,'','[]',0,0)", []any{a.rejudging, b.submission}},
			{"submission rejudging", "INSERT INTO rejudging_submissions(rejudging_id,submission_id,generation,prior_status,prior_score,prior_total_time_ms,prior_peak_memory_kb,prior_compile_result,prior_case_results,prior_judged_cases,prior_total_cases) VALUES($1,$2,1,'Accepted',100,0,0,'','[]',0,0)", []any{b.rejudging, a.submission}},
			{"scoreboard problem", "INSERT INTO contest_submission_cells(contest_id,problem_id,user_id) VALUES($1,$2,$3)", []any{a.contest, b.problem, user.ID}},
			{"scoreboard contest", "INSERT INTO contest_submission_cells(contest_id,problem_id,user_id) VALUES($1,$2,$3)", []any{b.contest, a.problem, user.ID}},
			{"clarification problem", "UPDATE clarifications SET problem_id=$2 WHERE id=$1", []any{a.clarification, b.problem}},
			{"clarification contest", "UPDATE clarifications SET contest_id=$2 WHERE id=$1", []any{a.clarification, b.contest}},
			{"clarification parent", "UPDATE clarifications SET parent_id=$2 WHERE id=$1", []any{a.clarification, b.clarification}},
		}
		for _, test := range cases {
			_, err := integrationDB.Pool.ExecContext(ctx, test.query, test.args...)
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue(), "%s: %v", test.name, err)
			Expect(pgErr.Code).To(Equal("23503"), test.name)
		}
		// Public-number allocation is local to the kind as well as the domain.
		for _, table := range []string{"contests", "submissions", "problem_sets", "editorials"} {
			var numbers []int64
			Expect(integrationDB.Pool.SelectContext(ctx, &numbers, fmt.Sprintf("SELECT public_id FROM %s ORDER BY domain_id", table))).To(Succeed())
			Expect(numbers).To(Equal([]int64{1, 1}), table)
		}
	})
})
