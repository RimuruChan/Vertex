package e2e

import (
	"fmt"
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
	ID       string `json:"id"`
	PublicID string `json:"publicId"`
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
	uploadTestdataAt(t, prefix+"/admin/problems/"+p.ID+"/testdata", owner, map[string]string{"1.in": "1 2\n", "1.out": "3\n"})
	var workspace workspaceResponse
	apiCall(t, http.MethodGet, prefix+"/admin/problems/"+p.ID+"/package", owner, nil, &workspace, 200)
	apiCall(t, http.MethodPost, prefix+"/admin/problems/"+p.ID+"/publish", owner, map[string]any{"revision": workspace.Meta.PackageRevision, "artifactVersion": workspace.Meta.TestdataVersion}, nil, 200)
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
	apiCall(t, http.MethodGet, prefix+"/problems/"+p.PublicID, reader, nil, nil, 404)
	var group numberedResource
	apiCall(t, http.MethodPost, prefix+"/groups", owner, map[string]string{"name": "Collaborators"}, &group, 201)
	apiCall(t, http.MethodPut, prefix+"/groups/"+group.PublicID+"/members/"+readerName, owner, map[string]string{"role": "member"}, nil, 200)
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+p.ID+"/access", owner, map[string]string{"group": group.PublicID, "role": "reader"}, nil, 200)
	apiCall(t, http.MethodGet, prefix+"/admin/problems/"+p.PublicID+"/package", reader, nil, nil, 200)
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
	apiCall(t, http.MethodDelete, prefix+"/groups/"+group.PublicID+"/members/"+readerName, owner, nil, nil, 200)
	apiCall(t, http.MethodGet, prefix+"/admin/problems/"+p.PublicID+"/package", reader, nil, nil, 403)

	var notice numberedResource
	apiCall(t, http.MethodPost, prefix+"/admin/announcements", owner, map[string]any{"title": "Draft notice", "published": false}, &notice, 201)
	apiCall(t, http.MethodGet, prefix+"/announcements/"+notice.PublicID, owner, nil, nil, 404)
	apiCall(t, http.MethodGet, prefix+"/admin/announcements/"+notice.PublicID, reader, nil, nil, 403)
	apiCall(t, http.MethodPut, prefix+"/admin/announcements/"+notice.ID, owner, map[string]any{"title": "Released notice", "published": true}, nil, 200)
	apiCall(t, http.MethodGet, prefix+"/announcements/"+notice.PublicID, reader, nil, nil, 200)
	var tag struct {
		ID int64 `json:"id"`
	}
	apiCall(t, http.MethodPost, prefix+"/admin/tags", owner, map[string]string{"name": "e2e-taxonomy"}, &tag, 201)
	apiCall(t, http.MethodPut, fmt.Sprintf("%s/admin/tags/%d", prefix, tag.ID), reader, map[string]string{"name": "denied"}, nil, 403)
	apiCall(t, http.MethodPut, fmt.Sprintf("%s/admin/tags/%d", prefix, tag.ID), owner, map[string]string{"name": "e2e-renamed"}, nil, 200)

	target := newDomain(t, base, owner)
	targetPrefix := scopedPrefix(base, target)
	var copied struct {
		ProblemID       string `json:"problemId"`
		ProblemPublicID string `json:"problemPublicId"`
		DomainID        string `json:"domainId"`
	}
	apiCall(t, http.MethodPost, targetPrefix+"/problem-copies", owner, map[string]any{"sourceDomain": space.Slug, "sourceProblem": p.PublicID, "sourceVersion": 1, "attribution": "Explicit E2E fixture copy"}, &copied, 201)
	if copied.DomainID != target.ID || copied.ProblemID == p.ID {
		t.Fatal("copy reused source identity or wrong domain")
	}
	apiCall(t, http.MethodGet, prefix+"/admin/problems/"+copied.ProblemID, owner, nil, nil, 404)
	apiCall(t, http.MethodDelete, prefix+"/admin/problems/"+p.ID, owner, nil, nil, 200)
	apiCall(t, http.MethodGet, targetPrefix+"/admin/problems/"+copied.ProblemPublicID+"/origin", owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, targetPrefix+"/admin/problems/"+copied.ProblemID, owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, prefix+"/admin/announcements/"+notice.ID, owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, fmt.Sprintf("%s/admin/tags/%d", prefix, tag.ID), owner, nil, nil, 200)
	apiCall(t, http.MethodDelete, prefix+"/groups/"+group.PublicID, owner, nil, nil, 200)
	apiCall(t, http.MethodPut, prefix+"/archive", owner, map[string]bool{"archived": true}, nil, 200)
	apiCall(t, http.MethodPut, targetPrefix+"/archive", owner, map[string]bool{"archived": true}, nil, 200)
}

// Workers must be stopped for this protocol fixture. The result is explicitly
// System Error: no submission or build program is executed by this test.
func TestEndToEndDomainProtocol(t *testing.T) {
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
	if job.DomainID != space.ID || job.ProblemVersion != 1 || job.SubmissionID != submitted.ID {
		t.Fatal("judge claim did not preserve domain, release and submission identity")
	}
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/jobs/"+job.JobID+"/heartbeat", serviceToken, map[string]any{"workerId": "wrong-worker", "generation": job.Generation, "leaseToken": job.LeaseToken, "judgedCases": 0}, nil, 409)
	apiCall(t, http.MethodPut, base+"/internal/judge/v1/jobs/"+job.JobID+"/result", serviceToken, map[string]any{"workerId": worker, "submissionId": job.SubmissionID, "generation": job.Generation, "leaseToken": job.LeaseToken, "status": "System Error", "score": 0, "compileResult": "Protocol fixture: no program executed"}, nil, 204)

	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+p.ID+"/files", owner, map[string]any{"kind": "solution", "name": "main.cpp", "language": "cpp", "sourceCode": "int main(){return 0;}", "isActive": true, "expectedVerdict": "Accepted"}, nil, 200)
	apiCall(t, http.MethodPost, prefix+"/admin/problems/"+p.ID+"/tests", owner, map[string]any{"source": "manual", "inputData": "1 2\n"}, nil, 201)
	var build buildResponse
	var sealed workspaceResponse
	apiCall(t, http.MethodGet, prefix+"/admin/problems/"+p.ID+"/package", owner, nil, &sealed, 200)
	apiCall(t, http.MethodPost, prefix+"/admin/problems/"+p.ID+"/builds", owner, nil, &build, 202)
	apiCall(t, http.MethodPut, prefix+"/admin/problems/"+p.ID+"/files", owner, map[string]any{"kind": "solution", "name": "main.cpp", "language": "cpp", "sourceCode": "int main(){return 1;}", "isActive": true, "expectedVerdict": "Accepted"}, nil, 200)
	var leased struct {
		BuildID      string `json:"buildId"`
		DomainID     string `json:"domainId"`
		DataRevision int    `json:"dataRevision"`
		Revision     int    `json:"revision"`
		LeaseToken   string `json:"leaseToken"`
		Solutions    []struct {
			SourceCode string `json:"sourceCode"`
		} `json:"solutions"`
	}
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/builds/claim", serviceToken, map[string]any{"workerId": worker, "waitSeconds": 1}, &leased, 200)
	if leased.BuildID != build.ID || leased.DomainID != space.ID || leased.DataRevision != sealed.Meta.DataRevision || leased.Revision != sealed.Meta.PackageRevision || len(leased.Solutions) != 1 || leased.Solutions[0].SourceCode != "int main(){return 0;}" {
		t.Fatal("build claim lost domain or sealed source snapshot")
	}
	apiCall(t, http.MethodPost, prefix+"/admin/problems/"+p.ID+"/builds/"+build.ID+"/cancel", owner, nil, nil, 200)
	apiCall(t, http.MethodPost, base+"/internal/judge/v1/builds/"+build.ID+"/progress", serviceToken, map[string]any{"workerId": worker, "leaseToken": leased.LeaseToken, "stage": "compiling"}, nil, 409)
	apiCall(t, http.MethodPut, prefix+"/archive", owner, map[string]bool{"archived": true}, nil, 200)
	t.Log("Verified API job context and fencing; no sandbox execution was performed")
}
