package e2e

// 社区与后台端到端测试:题单进度、题解防剧透与投票、讨论编辑、封禁账号、
// 标签合并与站点公告。

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

type setItem struct {
	ProblemID  string `json:"problemId"`
	Note       string `json:"note"`
	Title      string `json:"title"`
	UserStatus string `json:"userStatus"`
}

type problemSet struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Visibility   string    `json:"visibility"`
	ProblemCount int       `json:"problemCount"`
	SolvedCount  int       `json:"solvedCount"`
	CanEdit      bool      `json:"canEdit"`
	Items        []setItem `json:"items"`
}

type editorial struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	ContentMD string `json:"contentMd"`
	Locked    bool   `json:"locked"`
	VoteCount int    `json:"voteCount"`
	Voted     bool   `json:"voted"`
	CanEdit   bool   `json:"canEdit"`
}

type discussionPost struct {
	ID        int64  `json:"id"`
	ContentMD string `json:"contentMd"`
	Edited    bool   `json:"edited"`
}

type accountRow struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

type siteStats struct {
	Users       int `json:"users"`
	Problems    int `json:"problems"`
	ProblemSets int `json:"problemSets"`
	QueuedJobs  int `json:"queuedJobs"`
}

// TestEndToEndProblemSets covers curation, visibility and progress.
func TestEndToEndProblemSets(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminUser, adminPass := os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E_ADMIN_USER/E2E_ADMIN_PASS not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)
	problemID := contestProblem(t, base, admin, fmt.Sprintf("set-%d", time.Now().UnixNano()%1000000))

	curator, curatorName := registerUser(t, base)
	reader, _ := registerUser(t, base)

	var created problemSet
	if err := httpJSON(http.MethodPost, base+"/api/problem-sets", curator,
		map[string]any{"title": "入门题单", "description": "先做这些"}, &created, 201); err != nil {
		t.Fatalf("create problem set: %v", err)
	}
	if !created.CanEdit {
		t.Fatal("the curator cannot edit their own set")
	}
	if err := httpJSON(http.MethodPut, base+"/api/problem-sets/"+created.ID+"/items", curator,
		map[string]any{"items": []map[string]any{{"problemId": problemID, "note": "热身"}}},
		nil, 200); err != nil {
		t.Fatalf("set items: %v", err)
	}

	// A reader sees the set but cannot change it.
	var seen problemSet
	if err := httpJSON(http.MethodGet, base+"/api/problem-sets/"+created.ID, reader, nil, &seen, 200); err != nil {
		t.Fatalf("read problem set: %v", err)
	}
	if seen.CanEdit {
		t.Fatal("a reader was told they may edit the set")
	}
	if len(seen.Items) != 1 || seen.Items[0].Note != "热身" {
		t.Fatalf("set items = %+v", seen.Items)
	}
	if seen.SolvedCount != 0 {
		t.Fatalf("solved count = %d before solving anything", seen.SolvedCount)
	}
	if err := httpJSON(http.MethodPut, base+"/api/problem-sets/"+created.ID, reader,
		map[string]any{"title": "劫持"}, nil, http.StatusForbidden); err != nil {
		t.Fatalf("a reader was allowed to edit the set: %v", err)
	}

	// Solving the problem moves the progress counter.
	accepted := submit(t, base, reader, problemID, "cpp", acceptSolution)
	assertVerdict(t, waitForSubmission(t, base, reader, accepted, 3*time.Minute), "Accepted")
	if err := httpJSON(http.MethodGet, base+"/api/problem-sets/"+created.ID, reader, nil, &seen, 200); err != nil {
		t.Fatalf("re-read problem set: %v", err)
	}
	if seen.SolvedCount != 1 || seen.Items[0].UserStatus != "solved" {
		t.Fatalf("progress did not update: %+v", seen)
	}

	// A private set disappears for everyone but its curator.
	if err := httpJSON(http.MethodPut, base+"/api/problem-sets/"+created.ID, curator,
		map[string]any{"title": "入门题单", "visibility": "private"}, nil, 200); err != nil {
		t.Fatalf("make the set private: %v", err)
	}
	if err := httpJSON(http.MethodGet, base+"/api/problem-sets/"+created.ID, reader,
		nil, nil, http.StatusNotFound); err != nil {
		t.Fatalf("a private set was visible to a reader: %v", err)
	}
	if err := httpJSON(http.MethodGet, base+"/api/problem-sets/"+created.ID, curator, nil, &seen, 200); err != nil {
		t.Fatalf("the curator lost access to their own private set: %v", err)
	}
	_ = curatorName
}

// TestEndToEndCommunityContent covers the editorial spoiler gate, voting and
// discussion editing.
func TestEndToEndCommunityContent(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminUser, adminPass := os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E_ADMIN_USER/E2E_ADMIN_PASS not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)
	problemID := contestProblem(t, base, admin, fmt.Sprintf("editorial-%d", time.Now().UnixNano()%1000000))

	author, _ := registerUser(t, base)
	reader, _ := registerUser(t, base)

	var published editorial
	if err := httpJSON(http.MethodPost, base+"/api/editorials", author, map[string]any{
		"problemId": problemID, "title": "前缀和做法",
		"contentMd": "先求前缀和,再枚举右端点。", "solvedOnly": true,
	}, &published, 201); err != nil {
		t.Fatalf("publish editorial: %v", err)
	}

	// A reader who has not solved the problem sees the entry but not the body.
	var locked editorial
	if err := httpJSON(http.MethodGet, base+"/api/editorials/"+published.ID, reader, nil, &locked, 200); err != nil {
		t.Fatalf("read editorial as an unsolved reader: %v", err)
	}
	if !locked.Locked || locked.ContentMD != "" {
		t.Fatalf("the spoiler gate leaked the body: %+v", locked)
	}
	if locked.Title == "" {
		t.Fatal("the gated editorial hid its title, so nobody would know it exists")
	}

	// The author always reads their own text.
	var own editorial
	if err := httpJSON(http.MethodGet, base+"/api/editorials/"+published.ID, author, nil, &own, 200); err != nil {
		t.Fatalf("read own editorial: %v", err)
	}
	if own.Locked || own.ContentMD == "" {
		t.Fatalf("the author was locked out of their own editorial: %+v", own)
	}

	// Solving the problem unlocks it.
	accepted := submit(t, base, reader, problemID, "cpp", acceptSolution)
	assertVerdict(t, waitForSubmission(t, base, reader, accepted, 3*time.Minute), "Accepted")
	var unlocked editorial
	if err := httpJSON(http.MethodGet, base+"/api/editorials/"+published.ID, reader, nil, &unlocked, 200); err != nil {
		t.Fatalf("read editorial after solving: %v", err)
	}
	if unlocked.Locked || unlocked.ContentMD == "" {
		t.Fatalf("solving the problem did not unlock the editorial: %+v", unlocked)
	}

	// Voting is idempotent: a repeat vote does not inflate the count.
	for i := 0; i < 2; i++ {
		if err := httpJSON(http.MethodPost, base+"/api/editorials/"+published.ID+"/vote",
			reader, map[string]any{"up": true}, nil, 200); err != nil {
			t.Fatalf("vote: %v", err)
		}
	}
	if err := httpJSON(http.MethodGet, base+"/api/editorials/"+published.ID, reader, nil, &unlocked, 200); err != nil {
		t.Fatalf("re-read editorial: %v", err)
	}
	if unlocked.VoteCount != 1 || !unlocked.Voted {
		t.Fatalf("vote count = %d, voted = %v; want 1 and true", unlocked.VoteCount, unlocked.Voted)
	}
	if err := httpJSON(http.MethodPost, base+"/api/editorials/"+published.ID+"/vote",
		reader, map[string]any{"up": false}, nil, 200); err != nil {
		t.Fatalf("withdraw vote: %v", err)
	}
	if err := httpJSON(http.MethodGet, base+"/api/editorials/"+published.ID, reader, nil, &unlocked, 200); err != nil {
		t.Fatalf("re-read editorial after withdrawing: %v", err)
	}
	if unlocked.VoteCount != 0 {
		t.Fatalf("vote count = %d after withdrawing, want 0", unlocked.VoteCount)
	}

	// A reader may not edit or delete someone else's editorial.
	if err := httpJSON(http.MethodDelete, base+"/api/editorials/"+published.ID, reader,
		nil, nil, http.StatusForbidden); err != nil {
		t.Fatalf("a reader was allowed to delete another author's editorial: %v", err)
	}

	// Discussion: post, edit, and reject an edit from someone else.
	var post discussionPost
	if err := httpJSON(http.MethodPost, base+"/api/problems/"+problemID+"/discussions",
		reader, map[string]any{"contentMd": "样例二怎么理解?"}, &post, 201); err != nil {
		t.Fatalf("create discussion post: %v", err)
	}
	if post.Edited {
		t.Fatal("a brand new post was already marked as edited")
	}
	if err := httpJSON(http.MethodPut, fmt.Sprintf("%s/api/discussions/%d", base, post.ID),
		author, map[string]any{"contentMd": "劫持"}, nil, http.StatusForbidden); err != nil {
		t.Fatalf("another user was allowed to rewrite the post: %v", err)
	}
	var edited discussionPost
	if err := httpJSON(http.MethodPut, fmt.Sprintf("%s/api/discussions/%d", base, post.ID),
		reader, map[string]any{"contentMd": "想明白了,忽略这条。"}, &edited, 200); err != nil {
		t.Fatalf("edit own post: %v", err)
	}
	if edited.ContentMD != "想明白了,忽略这条。" {
		t.Fatalf("post content = %q", edited.ContentMD)
	}
	// An administrator moderates by deleting, which is allowed.
	if err := httpJSON(http.MethodDelete, fmt.Sprintf("%s/api/discussions/%d", base, post.ID),
		admin, nil, nil, 200); err != nil {
		t.Fatalf("admin could not remove a post: %v", err)
	}
}

// TestEndToEndAdminConsole covers the dashboard, account blocking, tag
// maintenance and announcements.
func TestEndToEndAdminConsole(t *testing.T) {
	base := os.Getenv("E2E_BASE_URL")
	if base == "" {
		t.Skip("E2E_BASE_URL not set")
	}
	adminUser, adminPass := os.Getenv("E2E_ADMIN_USER"), os.Getenv("E2E_ADMIN_PASS")
	if adminUser == "" || adminPass == "" {
		t.Skip("E2E_ADMIN_USER/E2E_ADMIN_PASS not set")
	}
	admin := mustLogin(t, base, adminUser, adminPass)

	var stats siteStats
	if err := httpJSON(http.MethodGet, base+"/api/admin/stats", admin, nil, &stats, 200); err != nil {
		t.Fatalf("read site stats: %v", err)
	}
	if stats.Users < 1 || stats.Problems < 0 {
		t.Fatalf("implausible statistics: %+v", stats)
	}

	// A plain user cannot reach the console.
	plain, plainName := registerUser(t, base)
	if err := httpJSON(http.MethodGet, base+"/api/admin/stats", plain, nil, nil, http.StatusForbidden); err != nil {
		t.Fatalf("a plain user reached the admin console: %v", err)
	}

	// Find the new account and block it.
	var accounts struct {
		Items []accountRow `json:"items"`
	}
	if err := httpJSON(http.MethodGet, base+"/api/admin/users?keyword="+plainName,
		admin, nil, &accounts, 200); err != nil {
		t.Fatalf("search accounts: %v", err)
	}
	if len(accounts.Items) == 0 {
		t.Fatalf("the new account %q is not in the admin list", plainName)
	}
	target := accounts.Items[0]
	if err := httpJSON(http.MethodPatch, base+"/api/admin/users/"+target.ID, admin,
		map[string]any{"disabled": true, "reason": "e2e"}, nil, 200); err != nil {
		t.Fatalf("block account: %v", err)
	}

	// The blocked account can no longer sign in, and its live token is dead.
	if err := httpJSON(http.MethodPost, base+"/api/auth/login", "",
		map[string]string{"username": plainName, "password": "testpass123"},
		nil, http.StatusForbidden); err != nil {
		t.Fatalf("a blocked account could still sign in: %v", err)
	}
	if err := httpJSON(http.MethodGet, base+"/api/submissions", plain, nil, nil, http.StatusUnauthorized); err != nil {
		t.Fatalf("a blocked account kept using its access token: %v", err)
	}

	// Unblocking restores access.
	if err := httpJSON(http.MethodPatch, base+"/api/admin/users/"+target.ID, admin,
		map[string]any{"disabled": false}, nil, 200); err != nil {
		t.Fatalf("unblock account: %v", err)
	}
	if err := httpJSON(http.MethodPost, base+"/api/auth/login", "",
		map[string]string{"username": plainName, "password": "testpass123"}, nil, 200); err != nil {
		t.Fatalf("an unblocked account could not sign in: %v", err)
	}

	// An administrator may not demote or block themselves.
	var self struct {
		Items []accountRow `json:"items"`
	}
	if err := httpJSON(http.MethodGet, base+"/api/admin/users?keyword="+adminUser,
		admin, nil, &self, 200); err != nil {
		t.Fatalf("search for the acting administrator: %v", err)
	}
	var selfID string
	for _, row := range self.Items {
		if row.Username == adminUser {
			selfID = row.ID
		}
	}
	if selfID == "" {
		t.Fatal("the acting administrator is not in the account list")
	}
	if err := httpJSON(http.MethodPatch, base+"/api/admin/users/"+selfID, admin,
		map[string]any{"role": "user"}, nil, http.StatusForbidden); err != nil {
		t.Fatalf("an administrator was allowed to demote themselves: %v", err)
	}
	if err := httpJSON(http.MethodPatch, base+"/api/admin/users/"+selfID, admin,
		map[string]any{"disabled": true}, nil, http.StatusForbidden); err != nil {
		t.Fatalf("an administrator was allowed to block themselves: %v", err)
	}

	// Announcements: drafts stay invisible to readers, published ones do not.
	var draft struct {
		ID string `json:"id"`
	}
	published := false
	if err := httpJSON(http.MethodPost, base+"/api/admin/announcements", admin,
		map[string]any{"title": "维护通知(草稿)", "contentMd": "稍后发布", "published": published},
		&draft, 201); err != nil {
		t.Fatalf("create draft announcement: %v", err)
	}
	var feed struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := httpJSON(http.MethodGet, base+"/api/announcements", "", nil, &feed, 200); err != nil {
		t.Fatalf("read the public announcement feed: %v", err)
	}
	for _, item := range feed.Items {
		if item.ID == draft.ID {
			t.Fatal("an unpublished announcement appeared in the public feed")
		}
	}
	if err := httpJSON(http.MethodPut, base+"/api/admin/announcements/"+draft.ID, admin,
		map[string]any{"title": "维护通知", "contentMd": "今晚 22:00 维护", "pinned": true},
		nil, 200); err != nil {
		t.Fatalf("publish announcement: %v", err)
	}
	if err := httpJSON(http.MethodGet, base+"/api/announcements", "", nil, &feed, 200); err != nil {
		t.Fatalf("re-read the announcement feed: %v", err)
	}
	found := false
	for _, item := range feed.Items {
		if item.ID == draft.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("a published announcement is missing from the public feed")
	}
	if err := httpJSON(http.MethodDelete, base+"/api/admin/announcements/"+draft.ID,
		admin, nil, nil, 200); err != nil {
		t.Fatalf("delete announcement: %v", err)
	}
}
