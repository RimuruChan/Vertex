package content

import (
	"context"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	"github.com/RimuruChan/Vertex/server/internal/problem"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Content input validation", func() {
	ctx := context.Background()
	It("trims editable fields and applies publication defaults", func() {
		repo := &fakeEditorialRepository{}
		_, err := NewService(repo, nil, &fakeScopeAccess{visible: true}).CreateEditorial(ctx, "author", false, EditorialInput{ProblemID: " p ", Title: " title ", ContentMD: " body "})
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.input).To(Equal(EditorialInput{ProblemID: "p", Title: "title", ContentMD: "body", Visibility: VisibilityPublic, Status: StatusPublished}))
	})
	It("rejects missing or oversized fields and unknown visibility or status", func() {
		service := NewService(&fakeEditorialRepository{}, nil, &fakeScopeAccess{visible: true})
		for _, input := range []EditorialInput{
			{Title: "t", ContentMD: "body"}, {ProblemID: "p", ContentMD: "body"}, {ProblemID: "p", Title: "t"},
			{ProblemID: "p", Title: strings.Repeat("t", maxEditorialTitle+1), ContentMD: "b"},
			{ProblemID: "p", Title: "t", ContentMD: strings.Repeat("b", maxEditorialContent+1)},
			{ProblemID: "p", Title: "t", ContentMD: "b", Visibility: "secret"},
			{ProblemID: "p", Title: "t", ContentMD: "b", Status: "review"},
		} {
			_, err := service.CreateEditorial(ctx, "author", false, input)
			Expect(err).To(MatchError(ErrInvalidInput))
		}
	})
	It("does not move an editorial while editing its content", func() {
		repo := &fakeEditorialRepository{item: &Editorial{ID: "e", ProblemID: "fixed", Permissions: Permissions{Edit: true}}}
		_, err := NewService(repo, nil, nil).UpdateEditorial(ctx, "e", "author", false, EditorialInput{ProblemID: "different", Title: "t", ContentMD: "body"})
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.input.ProblemID).To(Equal("fixed"))
	})
	It("fails closed when creation access is missing or unavailable", func() {
		input := EditorialInput{ProblemID: "p", Title: "t", ContentMD: "b"}
		repo := &fakeEditorialRepository{}
		_, err := NewService(repo, nil, &fakeScopeAccess{}).CreateEditorial(ctx, "u", true, input)
		Expect(err).To(MatchError(ErrNotFound))
		Expect(repo.input).To(Equal(EditorialInput{}))
		_, err = NewService(repo, nil, nil).CreateEditorial(ctx, "u", true, input)
		Expect(err).To(MatchError(ErrAccessUnavailable))
	})
	It("validates discussion bodies and identifiers before persistence", func() {
		service := NewService(nil, nil, nil)
		parent := int64(0)
		_, err := service.CreateProblemPost(ctx, "p", "u", false, "body", &parent)
		Expect(err).To(MatchError(ErrInvalidInput))
		for _, body := range []string{" ", strings.Repeat("b", maxPostContent+1)} {
			_, err := service.CreateEditorialPost(ctx, "e", "u", false, body, nil)
			Expect(err).To(MatchError(ErrInvalidInput))
			_, err = service.UpdatePost(ctx, 1, "u", body)
			Expect(err).To(MatchError(ErrInvalidInput))
		}
	})
	It("clamps editorial listing without rebuilding capabilities from caller roles", func() {
		repo := &fakeEditorialRepository{}
		_, _, err := NewService(repo, nil, nil).ListEditorials(ctx, EditorialFilters{Limit: 200, Keyword: " k ", Admin: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.filters.Limit).To(Equal(20))
		Expect(repo.filters.Keyword).To(Equal("k"))
	})
})

var _ = Describe("Content capabilities", func() {
	parent := func(userID string) problem.Access {
		return problem.Access{Scope: domain.Scope{Domain: domain.Domain{Visibility: "public"}, UserID: userID, MemberStatus: "active", RolePermissions: []domain.Permission{domain.CreateContent}}, OwnerID: "setter", Permissions: problem.Permissions{View: true}}
	}
	author := "author"
	It("never bypasses the parent boundary for authors or administrators", func() {
		p := parent(author)
		p.Permissions.View = false
		Expect(EditorialPermissions(p, &author, "private", "draft", false, false)).To(Equal(Permissions{}))
		p.Scope.SiteAdmin = true
		Expect(EditorialPermissions(p, &author, "public", "published", false, false)).To(Equal(Permissions{}))
		p = parent(author)
		p.Scope.MemberStatus = "suspended"
		Expect(EditorialPermissions(p, &author, "public", "published", false, false)).To(Equal(Permissions{}))
	})
	It("preserves author editing but limits moderators to removal", func() {
		owner := EditorialPermissions(parent(author), &author, "private", "draft", true, false)
		Expect(owner.Edit).To(BeTrue())
		Expect(owner.Delete).To(BeTrue())
		Expect(owner.Comment).To(BeFalse())
		p := parent("manager")
		p.Scope.RolePermissions = []domain.Permission{domain.ManageResources}
		manager := EditorialPermissions(p, &author, "private", "draft", true, false)
		Expect(manager.ViewBody).To(BeTrue())
		Expect(manager.Edit).To(BeFalse())
		Expect(manager.Delete).To(BeTrue())
		setter := EditorialPermissions(parent("setter"), &author, "public", "published", true, false)
		Expect(setter.Edit).To(BeFalse())
		Expect(setter.Delete).To(BeTrue())
		Expect(EditorialPermissions(parent("setter"), &author, "private", "draft", false, false).View).To(BeFalse())
	})
	It("requires practice completion before a reader can open, vote or discuss solved-only content", func() {
		locked := EditorialPermissions(parent("reader"), &author, "public", "published", true, false)
		Expect(locked.View).To(BeTrue())
		Expect(locked.ViewBody).To(BeFalse())
		Expect(locked.Comment).To(BeFalse())
		Expect(locked.Vote).To(BeFalse())
		unlocked := EditorialPermissions(parent("reader"), &author, "public", "published", true, true)
		Expect(unlocked.ViewBody && unlocked.Comment && unlocked.Vote).To(BeTrue())
	})
	It("does not equate ownership with permission to create new content", func() {
		p := parent(author)
		p.Scope.RolePermissions = nil
		value := EditorialPermissions(p, &author, "public", "published", false, false)
		Expect(value.Edit).To(BeTrue())
		Expect(value.Vote || value.Comment).To(BeFalse())
		p.Scope.Domain.Archived = true
		value = EditorialPermissions(p, &author, "public", "published", false, false)
		Expect(value.ViewBody).To(BeTrue())
		Expect(value.Edit || value.Delete).To(BeFalse())
	})
	It("keeps comment editing author-only even for moderators", func() {
		p := parent("manager")
		value := PostPermissions(p.Scope, true, true, &author)
		Expect(value.Edit).To(BeFalse())
		Expect(value.Delete).To(BeTrue())
		value = PostPermissions(parent(author).Scope, true, false, &author)
		Expect(value.Edit).To(BeTrue())
		Expect(PostPermissions(parent(author).Scope, false, true, &author)).To(Equal(Permissions{}))
	})
})
var _ = Describe("DiscussionPost.Edited", func() {
	It("distinguishes later edits from initial timestamps", func() {
		at := time.Now()
		post := DiscussionPost{CreatedAt: at, UpdatedAt: at}
		Expect(post.Edited()).To(BeFalse())
		post.UpdatedAt = at.Add(5 * time.Second)
		Expect(post.Edited()).To(BeTrue())
	})
})

type fakeScopeAccess struct{ visible bool }

func (f *fakeScopeAccess) CanViewProblem(context.Context, string, string, bool) (bool, error) {
	return f.visible, nil
}

type fakeEditorialRepository struct {
	item    *Editorial
	input   EditorialInput
	filters EditorialFilters
}

func (f *fakeEditorialRepository) List(_ context.Context, filters EditorialFilters) ([]EditorialSummary, int, error) {
	f.filters = filters
	return nil, 0, nil
}
func (f *fakeEditorialRepository) Get(context.Context, string, string) (*Editorial, error) {
	return f.item, nil
}
func (f *fakeEditorialRepository) Create(_ context.Context, _ string, input EditorialInput) (*Editorial, error) {
	f.input = input
	return &Editorial{}, nil
}
func (f *fakeEditorialRepository) Update(_ context.Context, _, _ string, input EditorialInput) (*Editorial, error) {
	f.input = input
	return &Editorial{}, nil
}
func (f *fakeEditorialRepository) Delete(context.Context, string, string) error { return nil }
func (f *fakeEditorialRepository) Vote(context.Context, string, string, bool) (int, error) {
	return 0, nil
}
