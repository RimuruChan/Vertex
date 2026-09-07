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
	"github.com/RimuruChan/Vertex/server/internal/contest"
	contesthandler "github.com/RimuruChan/Vertex/server/internal/contest/handler"
	"github.com/RimuruChan/Vertex/server/internal/database/dbtest"
	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/identity"
	"github.com/RimuruChan/Vertex/server/internal/middleware"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	"github.com/RimuruChan/Vertex/server/internal/publicid"
	"github.com/RimuruChan/Vertex/server/internal/ratelimit"
	"github.com/RimuruChan/Vertex/server/internal/transport/httpapi"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Publication HTTP boundaries", func() {
	It("requires the reviewed input and current resource rights for publication and contest adoption", func(ctx SpecContext) {
		if integrationDB == nil {
			Skip("TEST_DATABASE_URL is not configured")
		}
		Expect(dbtest.Reset(ctx, integrationDB, "TRUNCATE users RESTART IDENTITY CASCADE")).To(Succeed())
		users := map[string]string{}
		for _, name := range []string{"owner", "editor", "observer"} {
			u, err := identity.NewUserStore(integrationDB).Create(ctx, name, name+"@example.test", "fixture")
			Expect(err).NotTo(HaveOccurred())
			users[name] = u.ID
		}
		spaces := domain.NewService(domain.NewStore(integrationDB))
		scope, err := spaces.Create(ctx, users["owner"], domain.CreateInput{Slug: "publishing", Name: "Publishing"})
		Expect(err).NotTo(HaveOccurred())
		for _, name := range []string{"editor", "observer"} {
			Expect(spaces.SetMember(ctx, "publishing", users["owner"], domain.MemberInput{Username: name, RoleKey: "member", Status: "active"})).To(Succeed())
		}
		as := func(actor string) context.Context {
			return domain.WithScope(ctx, domain.Scope{Domain: scope.Domain, UserID: users[actor]})
		}
		root := GinkgoT().TempDir()
		writer := problem.NewProblemAdminStore(integrationDB, root)
		task, err := writer.Create(as("owner"), users["owner"], &problem.CreateInput{Title: "Reviewed task", StatementMD: "First statement", Visibility: "public"})
		Expect(err).NotTo(HaveOccurred())
		Expect(writer.SetGrant(as("owner"), task.ID, problem.GrantInput{Username: "editor", Role: problem.AccessEditor})).To(Succeed())
		packages := authoring.NewPackageStore(integrationDB)
		service, err := authoring.NewService(packages, authoring.NewBuildStore(integrationDB), authoring.NewTestdataPublisher(root), authoring.NewDispatcher(1), time.Minute, time.Second)
		Expect(err).NotTo(HaveOccurred())
		contests := contest.NewContestStore(integrationDB)
		auth := middleware.NewAuthMiddleware(staleRoleAuthenticator{users: users})
		router := gin.New()
		api := router.Group("/api/domains/:domain")
		resolve := middleware.ResolveDomain(spaces)
		numbers := httpapi.PublicIDs(publicid.NewStore(integrationDB))
		authoringhandler.RegisterRoutes(api, authoringhandler.NewPackageHandler(service), auth.Require(), resolve, numbers)
		contesthandler.NewContestHandler(contest.NewService(contests, nil), ratelimit.Policy{}).RegisterRoutes(api, auth.Optional(), auth.Require(), resolve, numbers)
		request := func(method, route, actor, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, "/api/domains/"+route, strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			if actor != "" {
				r.Header.Set("Authorization", "Bearer "+actor)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, r)
			return response
		}
		route := "publishing/admin/problems/" + task.PublicID
		for _, body := range []string{`{}`, `{"revision":null,"artifactVersion":1}`, `{"revision":-1,"artifactVersion":1}`, `{"revision":0,"artifactVersion":0}`} {
			Expect(request("POST", route+"/publish", "owner", body).Code).To(Equal(400), body)
		}
		Expect(request("POST", route+"/publish", "", `{"revision":0,"artifactVersion":1}`).Code).To(Equal(401))
		Expect(request("POST", route+"/publish", "owner", `{"revision":0,"artifactVersion":1}`).Code).To(Equal(409))
		Expect(request("POST", route+"/publish", "owner", `{"revision":0,"artifactVersion":1,"language":"`+strings.Repeat("x", 17<<10)+`"}`).Code).To(Equal(413))

		var archive bytes.Buffer
		zipWriter := zip.NewWriter(&archive)
		for name, data := range map[string]string{"1.in": "1\n", "1.out": "1\n"} {
			entry, err := zipWriter.Create(name)
			Expect(err).NotTo(HaveOccurred())
			_, err = entry.Write([]byte(data))
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(zipWriter.Close()).To(Succeed())
		_, _, err = writer.SaveTestdata(as("editor"), task.ID, archive.Bytes(), "diff")
		Expect(err).NotTo(HaveOccurred())
		meta, err := packages.Meta(as("owner"), task.ID)
		Expect(err).NotTo(HaveOccurred())
		input := fmt.Sprintf(`{"revision":%d,"artifactVersion":%d}`, meta.PackageRevision, meta.TestdataVersion)
		Expect(request("POST", route+"/publish", "editor", input).Code).To(Equal(403))
		Expect(request("POST", "official/admin/problems/"+task.ID+"/publish", "owner", input).Code).To(Equal(404))
		response := request("POST", route+"/publish", "owner", input)
		Expect(response.Code).To(Equal(200), response.Body.String())
		var release struct {
			Version int `json:"version"`
		}
		Expect(json.Unmarshal(response.Body.Bytes(), &release)).To(Succeed())
		Expect(release.Version).To(Equal(1))
		Expect(request("GET", route+"/releases", "editor", "").Code).To(Equal(200))

		event, err := contests.Create(as("owner"), users["owner"], &contest.PersistInput{Title: "Pinned", Rule: "icpc", Visibility: "private", Feedback: "full", BeginAt: time.Now().Add(time.Hour), EndAt: time.Now().Add(2 * time.Hour)})
		Expect(err).NotTo(HaveOccurred())
		Expect(contests.SetProblems(as("owner"), event.ID, []contest.ProblemEntry{{ProblemID: task.ID, Label: "A", Points: 100}})).To(Succeed())
		Expect(contests.SetGrant(as("owner"), event.ID, contest.GrantInput{Username: "observer", Role: contest.AccessObserver})).To(Succeed())
		_, err = packages.SaveStatement(as("editor"), authoring.Statement{ProblemID: task.ID, Language: "zh", Name: "Reviewed v2", Legend: "Second statement"})
		Expect(err).NotTo(HaveOccurred())
		Expect(request("POST", route+"/publish", "owner", input).Code).To(Equal(409))
		meta, err = packages.Meta(as("owner"), task.ID)
		Expect(err).NotTo(HaveOccurred())
		input = fmt.Sprintf(`{"revision":%d,"artifactVersion":%d}`, meta.PackageRevision, meta.TestdataVersion)
		Expect(request("POST", route+"/publish", "owner", input).Code).To(Equal(200))
		adopt := "publishing/contests/" + event.PublicID + "/problems/A/version"
		Expect(request("PUT", adopt, "owner", `{"version":2}`).Code).To(Equal(400))
		Expect(request("PUT", adopt, "observer", `{"version":2,"expectedVersion":1}`).Code).To(Equal(403))
		Expect(request("PUT", adopt, "owner", `{"version":2,"expectedVersion":1}`).Code).To(Equal(200))
		Expect(request("PUT", adopt, "owner", `{"version":1,"expectedVersion":1}`).Code).To(Equal(409))
		Expect(request("PUT", adopt, "owner", `{"version":99,"expectedVersion":2}`).Code).To(Equal(400))
		pinned, err := contests.Problem(as("owner"), event.ID, "A")
		Expect(err).NotTo(HaveOccurred())
		Expect(pinned.Version).To(Equal(2))
		Expect(pinned.Title).To(Equal("Reviewed v2"))
		Expect(spaces.SetMember(ctx, "publishing", users["owner"], domain.MemberInput{Username: "editor", RoleKey: "member", Status: "suspended"})).To(Succeed())
		Expect(request("GET", route+"/releases", "editor", "").Code).To(Equal(404))
	})
})
