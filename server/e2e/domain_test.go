package e2e

import (
	"encoding/json"
	"fmt"
	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type domainFixture struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}
type numberedResource struct {
	ID string `json:"id"`
}

func apiBase(t *testing.T) string {
	t.Helper()
	base := strings.TrimRight(os.Getenv("E2E_BASE_URL"), "/")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	return base
}
func apiCall(t *testing.T, method, endpoint, token string, body, out any, status int) {
	t.Helper()
	if err := httpJSON(method, endpoint, token, body, out, status); err != nil {
		t.Fatalf("%s %s: %v", method, strings.Split(endpoint, "?")[0], err)
	}
}
func newDomain(t *testing.T, base, owner string) domainFixture {
	t.Helper()
	var result domainFixture
	apiCall(t, http.MethodPost, base+"/api/domains", owner, map[string]any{"slug": "e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36), "name": "E2E domain", "visibility": "private", "joinPolicy": "invite"}, &result, 201)
	return result
}
func scopedPrefix(base string, space domainFixture) string {
	return base + "/api/domains/" + space.Slug
}
func releasedPrivateProblem(t *testing.T, prefix, owner string) numberedResource {
	t.Helper()
	var p numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/problems", owner, map[string]any{"title": "Private protocol fixture", "statementMd": "Fixture statement", "visibility": "private", "timeLimitMs": 1000, "memoryLimitKb": 262144}, &p, 201)
	saveFixtureDataAt(t, prefix+"/authoring/problems/"+p.ID, owner, map[string]string{"1.in": "1 2\n", "1.out": "3\n"})
	publishFixtureAt(t, prefix+"/authoring/problems/"+p.ID, owner)
	return p
}

// This suite needs a real Server/PostgreSQL and filesystem, but not a worker.
// It deliberately creates no judging or build jobs.
func TestEndToEndDomainWorkflow(t *testing.T) {
	base := apiBase(t)
	owner, _ := registerUser(t, base)
	reader, readerName := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	apiCall(t, http.MethodGet, prefix+"/problems", reader, nil, nil, 404)
	apiCall(t, http.MethodPut, prefix+"/members/"+readerName, owner, map[string]any{"roleKey": "member", "status": "active"}, nil, 200)
	p := releasedPrivateProblem(t, prefix, owner)
	apiCall(t, http.MethodGet, prefix+"/problems/"+p.ID, reader, nil, nil, 404)
	var group numberedResource
	apiCall(t, http.MethodPost, prefix+"/groups", owner, map[string]string{"name": "Collaborators"}, &group, 201)
	apiCall(t, http.MethodPut, prefix+"/groups/"+group.ID+"/members/"+readerName, owner, map[string]string{"role": "member"}, nil, 200)
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+p.ID+"/access", owner, map[string]string{"group": group.ID, "role": "reader"}, nil, 200)
	apiCall(t, http.MethodGet, prefix+"/authoring/problems/"+p.ID+"/commits/1", reader, nil, nil, 200)
	var available struct {
		Items []numberedResource `json:"items"`
		Total int                `json:"total"`
	}
	apiCall(t, http.MethodGet, prefix+"/problems?view=available", reader, nil, &available, 200)
	if available.Total != 1 || len(available.Items) != 1 || available.Items[0].ID != p.ID {
		t.Fatal("available view did not preserve private collaboration scope")
	}
	apiCall(t, http.MethodGet, prefix+"/problems", reader, nil, &available, 200)
	if available.Total != 0 {
		t.Fatal("private problem entered public library")
	}
	apiCall(t, http.MethodDelete, prefix+"/groups/"+group.ID+"/members/"+readerName, owner, nil, nil, 200)
	apiCall(t, http.MethodGet, prefix+"/authoring/problems/"+p.ID+"/commits/1", reader, nil, nil, 403)

	var notice numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/announcements", owner, map[string]any{"title": "Draft notice", "published": false}, &notice, 201)
	apiCall(t, http.MethodGet, prefix+"/announcements/"+notice.ID, owner, nil, nil, 404)
	apiCall(t, http.MethodGet, prefix+"/admin/announcements/"+notice.ID, reader, nil, nil, 403)
	apiCall(t, http.MethodPut, prefix+"/admin/announcements/"+notice.ID, owner, map[string]any{"title": "Released notice", "published": true}, nil, 200)
	apiCall(t, http.MethodGet, prefix+"/announcements/"+notice.ID, reader, nil, nil, 200)
	var tag struct {
		ID int64 `json:"id"`
	}
	apiCall(t, http.MethodPost, prefix+"/admin/tags", owner, map[string]string{"name": "e2e-taxonomy"}, &tag, 201)
	apiCall(t, http.MethodPut, fmt.Sprintf("%s/admin/tags/%d", prefix, tag.ID), reader, map[string]string{"name": "denied"}, nil, 403)
	apiCall(t, http.MethodPut, fmt.Sprintf("%s/admin/tags/%d", prefix, tag.ID), owner, map[string]string{"name": "e2e-renamed"}, nil, 200)

	target := newDomain(t, base, owner)
	targetPrefix := scopedPrefix(base, target)
	var copied struct {
		ProblemID string `json:"problemId"`
		DomainID  string `json:"domainId"`
	}
	apiCall(t, http.MethodPost, targetPrefix+"/authoring/problem-copies", owner, map[string]any{"sourceDomain": space.Slug, "sourceProblem": p.ID, "sourceVersion": 1, "attribution": "Explicit E2E fixture copy"}, &copied, 201)
	if copied.DomainID != target.ID || copied.DomainID == space.ID || copied.ProblemID == "" {
		t.Fatal("copy has no public identity or uses the wrong domain")
	}
	var sourceView struct {
		DomainID string `json:"domainId"`
	}
	apiCall(t, http.MethodGet, prefix+"/admin/problems/"+copied.ProblemID, owner, nil, &sourceView, 200)
	if sourceView.DomainID != space.ID {
		t.Fatal("same public number escaped its routed domain")
	}
	apiCall(t, http.MethodDelete, prefix+"/admin/problems/"+p.ID, owner, nil, nil, 200)
	apiCall(t, http.MethodGet, targetPrefix+"/authoring/problems/"+copied.ProblemID+"/origin", owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, targetPrefix+"/admin/problems/"+copied.ProblemID, owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, prefix+"/admin/announcements/"+notice.ID, owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, fmt.Sprintf("%s/admin/tags/%d", prefix, tag.ID), owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, prefix+"/groups/"+group.ID, owner, nil, nil, 200)
	apiCall(t, http.MethodPut, prefix+"/archive", owner, map[string]bool{"archived": true}, nil, 200)
	apiCall(t, http.MethodPut, targetPrefix+"/archive", owner, map[string]bool{"archived": true}, nil, 200)
}

// Workers must be stopped for this protocol fixture. The result is explicitly
// System Error: no submission or build program is executed by this test.
func TestEndToEndDomainProtocol(t *testing.T) {
	priorFixtureMode := fixtureChecksWithoutExecution
	fixtureChecksWithoutExecution = true
	defer func() { fixtureChecksWithoutExecution = priorFixtureMode }()
	base := apiBase(t)
	serviceToken := os.Getenv("E2E_JUDGE_API_TOKEN")
	if serviceToken == "" {
		t.Skip("E2E_JUDGE_API_TOKEN not set; requires stopped workers")
	}
	owner, _ := registerUser(t, base)
	space := newDomain(t, base, owner)
	prefix := scopedPrefix(base, space)
	p := releasedPrivateProblem(t, prefix, owner)
	var submitted submission
	apiCall(t, http.MethodPost, prefix+"/submissions", owner, map[string]string{"problemId": p.ID, "language": "cpp", "sourceCode": "int main(){return 0;}"}, &submitted, 202)
	var job struct {
		JobID          string `json:"jobId"`
		SubmissionID   string `json:"submissionId"`
		DomainID       string `json:"domainId"`
		ProblemVersion int    `json:"problemVersion"`
		Generation     int    `json:"generation"`
		LeaseToken     string `json:"leaseToken"`
	}
	const worker = "domain-protocol-fixture"
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/jobs/claim", serviceToken, map[string]any{"workerId": worker, "waitSeconds": 1, "capabilities": []string{"cpp"}}, &job, 200)
	if job.DomainID != space.ID || job.ProblemVersion != 1 || job.SubmissionID == submitted.ID || len(job.SubmissionID) != 36 {
		t.Fatal("judge claim did not preserve domain, release and submission identity")
	}
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/jobs/"+job.JobID+"/heartbeat", serviceToken, map[string]any{"workerId": "wrong-worker", "generation": job.Generation, "leaseToken": job.LeaseToken, "judgedCases": 0}, nil, 409)
	apiCall(t, http.MethodPut, base+"/internal/judge/v1/jobs/"+job.JobID+"/result", serviceToken, map[string]any{"workerId": worker, "submissionId": job.SubmissionID, "generation": job.Generation, "leaseToken": job.LeaseToken, "status": "System Error", "score": 0, "compileResult": "Protocol fixture: no program executed"}, nil, 204)
	var evaluated submission
	apiCall(t, http.MethodGet, prefix+"/submissions/"+submitted.ID, owner, nil, &evaluated, 200)
	if evaluated.Status != "System Error" {
		t.Fatal("worker result did not reach the tenant's public submission")
	}

	endpoint := prefix + "/authoring/problems/" + p.ID
	var copy domain.WorkingCopy
	apiCall(t, http.MethodGet, endpoint+"/working-copy", owner, nil, &copy, 200)
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/fixture-source/text", owner, map[string]any{"etag": copy.ETag, "text": "print(3)\n"}, &copy, 200)
	var build domain.CheckRun
	apiCall(t, http.MethodPost, endpoint+"/checks", owner, domain.CheckSelection{ETag: copy.ETag}, &build, 200)
	apiCall(t, http.MethodPut, endpoint+"/working-copy/entries/fixture-source/text", owner, map[string]any{"etag": copy.ETag, "text": "print(4)\n"}, &copy, 200)
	var leased struct {
		BuildID    string                `json:"buildId"`
		DomainID   string                `json:"domainId"`
		LeaseToken string                `json:"leaseToken"`
		Check      *domain.CheckSnapshot `json:"check"`
	}
	var wire map[string]json.RawMessage
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/builds/claim", serviceToken, map[string]any{"workerId": worker, "waitSeconds": 1}, nil, 400)
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/builds/claim", serviceToken, map[string]any{"workerId": worker, "waitSeconds": 1, "checkProtocol": domain.CheckPolicyVersion}, &wire, 200)
	for _, retired := range []string{"revision", "dataRevision", "generators", "solutions", "tests", "checker", "validator", "interactor", "timeLimitMs", "memoryLimitKb", "judgeType"} {
		if _, exists := wire[retired]; exists {
			t.Fatalf("claim still exposes retired inline field %q", retired)
		}
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &leased); err != nil {
		t.Fatal(err)
	}
	if leased.BuildID != build.ID || leased.DomainID != space.ID || leased.Check == nil || leased.Check.TreeHash != build.TreeHash {
		t.Fatal("claim lost sealed authoring source")
	}
	found := false
	for _, program := range leased.Check.Programs {
		for _, file := range program.Files {
			if file.ID == "fixture-source" {
				found = file.Blob.SHA256 == domain.Digest([]byte("print(3)\n"))
			}
		}
	}
	if !found {
		t.Fatal("claim read current source instead of frozen source")
	}
	apiCall(t, http.MethodPost, endpoint+"/checks/"+build.ID+"/cancel", owner, nil, nil, 200)
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/builds/"+build.ID+"/progress", serviceToken, map[string]any{"workerId": worker, "leaseToken": leased.LeaseToken, "stage": "compile"}, nil, 409)

	apiCall(t, http.MethodPut, prefix+"/archive", owner, map[string]bool{"archived": true}, nil, 200)
	t.Log("Verified API job context and fencing; no sandbox execution was performed")
}
