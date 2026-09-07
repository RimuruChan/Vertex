package domain_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/authoring"
	authoringhandler "github.com/RimuruChan/Vertex/server/internal/authoring/handler"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	problemhandler "github.com/RimuruChan/Vertex/server/internal/problem/handler"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Copy HTTP domain boundaries", func() {
	It("binds the destination path, checks both resources and keeps private provenance out of public responses", func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"setter", "copier", "reader"} {
			u, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces := domain.NewService(domain.NewStore(integrationDB))
		source, err := spaces.Create(ctx, users["setter"], domain.CreateInput{Slug: "private-source", Name: "Private source"})
		Expect(err).NotTo(HaveOccurred())
		Expect(spaces.SetMember(ctx, source.Domain.Slug, users["setter"], domain.MemberInput{Username: "copier", RoleKey: "member", Status: "active"})).To(Succeed())
		_, err = integrationDB.Pool.ExecContext(ctx, "UPDATE domain_members SET role_key='author' WHERE domain_id=$1 AND user_id=$2", domain.OfficialID, users["copier"])
		Expect(err).NotTo(HaveOccurred())
		as := func(spaceID, actor string) context.Context {
			return domain.WithScope(ctx, domain.Scope{Domain: domain.Domain{ID: spaceID}, UserID: users[actor]})
		}
		root := GinkgoT().TempDir()
		writer := problem.NewProblemAdminStore(integrationDB, root)
		task, err := writer.Create(as(source.Domain.ID, "setter"), users["setter"], &problem.CreateInput{Title: "Source release", StatementMD: "Statement", Visibility: "public", Source: "Public credit"})
		Expect(err).NotTo(HaveOccurred())
		var archive bytes.Buffer
		zipWriter := zip.NewWriter(&archive)
		for name, body := range map[string]string{"1.in": "1\n", "1.out": "1\n"} {
			file, err := zipWriter.Create(name)
			Expect(err).NotTo(HaveOccurred())
			_, err = file.Write([]byte(body))
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(zipWriter.Close()).To(Succeed())
		_, _, err = writer.SaveTestdata(as(source.Domain.ID, "setter"), task.ID, archive.Bytes(), "diff")
		Expect(err).NotTo(HaveOccurred())
		packages := authoring.NewPackageStore(integrationDB)
		meta, err := packages.Meta(as(source.Domain.ID, "setter"), task.ID)
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Publish(as(source.Domain.ID, "setter"), task.ID, authoring.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		service, err := authoring.NewService(packages, authoring.NewBuildStore(integrationDB), authoring.NewTestdataPublisher(root), authoring.NewDispatcher(1), time.Minute, time.Second)
		Expect(err).NotTo(HaveOccurred())
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		authoringhandler.RegisterCopyRoutes(router.Group("/api"), authoringhandler.NewPackageHandler(service), auth.Require(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)))
		reader := problemhandler.NewProblemHandler(problem.NewService(problem.NewProblemStore(integrationDB), writer))
		router.GET("/api/domains/:domain/problems/:id", auth.Optional(), middleware.ResolveDomain(spaces), httpapi.PublicIDs(publicid.NewStore(integrationDB)), reader.Get)
		request := func(method, route, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/"+route, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			result := httptest.NewRecorder()
			router.ServeHTTP(result, r)
			return result
		}
		body := fmt.Sprintf(`{"sourceDomain":"private-source","sourceProblem":"%s","sourceVersion":1,"attribution":"Approved training copy","domainId":"%s","ownerId":"%s"}`, task.PublicID, source.Domain.ID, users["setter"])
		Expect(request("POST", "official/problem-copies", "", body).Code).To(Equal(401))
		Expect(request("POST", "official/problem-copies", "copier", `{}`).Code).To(Equal(400))
		Expect(request("POST", "official/problem-copies", "copier", `{"attribution":"`+strings.Repeat("x", 17<<10)+`"}`).Code).To(Equal(413))
		Expect(request("POST", "official/problem-copies", "copier", body).Code).To(Equal(403))
		Expect(writer.SetGrant(as(source.Domain.ID, "setter"), task.ID, problem.GrantInput{Username: "copier", Role: problem.AccessReader})).To(Succeed())
		wrongSource := fmt.Sprintf(`{"sourceDomain":"official","sourceProblem":"%s","sourceVersion":1,"attribution":"Wrong domain"}`, task.ID)
		Expect(request("POST", "official/problem-copies", "copier", wrongSource).Code).To(Equal(404))
		response := request("POST", "official/problem-copies", "copier", body)
		Expect(response.Code).To(Equal(201), response.Body.String())
		var created struct {
			ProblemID string `json:"problemId"`
			PublicID  string `json:"problemPublicId"`
			DomainID  string `json:"domainId"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &created)).To(Succeed())
		Expect(created.DomainID).To(Equal(domain.OfficialID))
		copied, err := problem.NewProblemStore(integrationDB).GetWorkspace(as(domain.OfficialID, "copier"), created.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		Expect(copied.OwnerID).To(Equal(users["copier"]))
		Expect(copied.Visibility).To(Equal("draft"))
		originRoute := "official/admin/problems/" + created.PublicID + "/origin"
		response = request("GET", originRoute, "copier", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring("private-source"))
		response = request("GET", originRoute, "reader", "")
		Expect(response.Code).To(Equal(403))
		Expect(response.Body.String()).NotTo(ContainSubstring("private-source"))
		Expect(request("GET", "private-source/admin/problems/"+created.ProblemID+"/origin", "setter", "").Code).To(Equal(404))
		Expect(spaces.SetMember(ctx, source.Domain.Slug, users["setter"], domain.MemberInput{Username: "copier", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(request("POST", "official/problem-copies", "copier", body).Code).To(Equal(404))
		Expect(request("GET", originRoute, "copier", "").Code).To(Equal(200))
		Expect(copied.Source).To(Equal("Public credit"))
		_, err = writer.Update(as(domain.OfficialID, "copier"), created.ProblemID, &problem.UpdateInput{CreateInput: problem.CreateInput{Title: copied.Title, StatementMD: copied.StatementMD, Source: copied.Source, Visibility: "public", TimeLimitMs: copied.TimeLimitMs, MemoryLimitKb: copied.MemoryLimitKb, Difficulty: copied.Difficulty, Tags: copied.Tags}})
		Expect(err).NotTo(HaveOccurred())
		meta, err = packages.Meta(as(domain.OfficialID, "copier"), created.ProblemID)
		Expect(err).NotTo(HaveOccurred())
		_, err = packages.Publish(as(domain.OfficialID, "copier"), created.ProblemID, authoring.PublishInput{Revision: meta.PackageRevision, ArtifactVersion: meta.TestdataVersion})
		Expect(err).NotTo(HaveOccurred())
		response = request("GET", "official/problems/"+created.PublicID, "", "")
		Expect(response.Code).To(Equal(200))
		Expect(response.Body.String()).To(ContainSubstring("Public credit"))
		Expect(response.Body.String()).NotTo(ContainSubstring("private-source"))
		Expect(response.Body.String()).NotTo(ContainSubstring("Approved training copy"))
	})
})
