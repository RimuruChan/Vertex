package application

import (
	"context"
	"strings"
	"testing"
	"time"

	contentdomain "github.com/RimuruChan/Vertex/server/internal/content/domain"
	problemdomain "github.com/RimuruChan/Vertex/server/internal/problem/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Content input validation", func() {
	ctx := context.Background()
	It("trims editable fields and applies publication defaults", func() {
		repo := &fakeEditorialRepository{}
		_, err := NewService(repo, nil, &fakeScopeAccess{visible: true}).CreateEditorial(ctx, "author", contentdomain.EditorialInput{ProblemID: " p ", Title: " title ", ContentMD: " body "})
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.input).To(Equal(contentdomain.EditorialInput{ProblemID: "p", Title: "title", ContentMD: "body", Visibility: contentdomain.VisibilityPublic, Status: contentdomain.StatusPublished}))
	})
	It("rejects missing or oversized fields and unknown visibility or status", func() {
		service := NewService(&fakeEditorialRepository{}, nil, &fakeScopeAccess{visible: true})
		for _, input := range []contentdomain.EditorialInput{
			{Title: "t", ContentMD: "body"}, {ProblemID: "p", ContentMD: "body"}, {ProblemID: "p", Title: "t"},
			{ProblemID: "p", Title: strings.Repeat("t", contentdomain.MaxEditorialTitleBytes+1), ContentMD: "b"},
			{ProblemID: "p", Title: "t", ContentMD: strings.Repeat("b", contentdomain.MaxEditorialBodyBytes+1)},
			{ProblemID: "p", Title: "t", ContentMD: "b", Visibility: "secret"},
			{ProblemID: "p", Title: "t", ContentMD: "b", Status: "review"},
		} {
			_, err := service.CreateEditorial(ctx, "author", input)
			Expect(err).To(MatchError(contentdomain.ErrInvalidInput))
		}
	})
	It("does not move an editorial while editing its content", func() {
		repo := &fakeEditorialRepository{item: &contentdomain.Editorial{ID: "e", ProblemID: "fixed", Permissions: contentdomain.Permissions{Edit: true}}}
		_, err := NewService(repo, nil, nil).UpdateEditorial(ctx, "e", "author", contentdomain.EditorialInput{ProblemID: "different", Title: "t", ContentMD: "body"})
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.input.ProblemID).To(Equal("fixed"))
	})
	It("fails closed when creation access is missing or unavailable", func() {
		input := contentdomain.EditorialInput{ProblemID: "p", Title: "t", ContentMD: "b"}
		repo := &fakeEditorialRepository{}
		_, err := NewService(repo, nil, &fakeScopeAccess{}).CreateEditorial(ctx, "u", input)
		Expect(err).To(MatchError(contentdomain.ErrNotFound))
		Expect(repo.input).To(Equal(contentdomain.EditorialInput{}))
		_, err = NewService(repo, nil, nil).CreateEditorial(ctx, "u", input)
		Expect(err).To(MatchError(contentdomain.ErrAccessUnavailable))
	})
	It("validates discussion bodies and identifiers before persistence", func() {
		service := NewService(nil, nil, nil)
		parent := int64(0)
		_, err := service.CreateProblemPost(ctx, "p", "u", "body", &parent)
		Expect(err).To(MatchError(contentdomain.ErrInvalidInput))
		for _, body := range []string{" ", strings.Repeat("b", contentdomain.MaxPostBodyBytes+1)} {
			_, err := service.CreateEditorialPost(ctx, "e", "u", body, nil)
			Expect(err).To(MatchError(contentdomain.ErrInvalidInput))
			_, err = service.UpdatePost(ctx, 1, "u", body)
			Expect(err).To(MatchError(contentdomain.ErrInvalidInput))
		}
	})
	It("clamps editorial listing without rebuilding capabilities from caller roles", func() {
		repo := &fakeEditorialRepository{}
		_, _, err := NewService(repo, nil, nil).ListEditorials(ctx, contentdomain.EditorialFilters{Limit: 200, Keyword: " k "})
		Expect(err).NotTo(HaveOccurred())
		Expect(repo.filters.Limit).To(Equal(20))
		Expect(repo.filters.Keyword).To(Equal("k"))
	})
})

var _ = Describe("Content capabilities", func() {
	parent := func(userID string) problemdomain.Access {
		return problemdomain.Access{Scope: tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public"}, UserID: userID, MemberStatus: "active", RolePermissions: []tenancydomain.Permission{tenancydomain.CreateContent}}, OwnerID: "setter", Permissions: problemdomain.Permissions{View: true}}
	}
	author := "author"
	It("never bypasses the parent boundary for authors or administrators", func() {
		p := parent(author)
		p.Permissions.View = false
		Expect(contentdomain.EditorialPermissions(p, &author, "private", "draft", false, false)).To(Equal(contentdomain.Permissions{}))
		p.Scope.SiteAdmin = true
		Expect(contentdomain.EditorialPermissions(p, &author, "public", "published", false, false)).To(Equal(contentdomain.Permissions{}))
		p = parent(author)
		p.Scope.MemberStatus = "suspended"
		Expect(contentdomain.EditorialPermissions(p, &author, "public", "published", false, false)).To(Equal(contentdomain.Permissions{}))
	})
	It("preserves author editing but limits moderators to removal", func() {
		owner := contentdomain.EditorialPermissions(parent(author), &author, "private", "draft", true, false)
		Expect(owner.Edit).To(BeTrue())
		Expect(owner.Delete).To(BeTrue())
		Expect(owner.Comment).To(BeFalse())
		p := parent("manager")
		p.Scope.RolePermissions = []tenancydomain.Permission{tenancydomain.ManageResources}
		manager := contentdomain.EditorialPermissions(p, &author, "private", "draft", true, false)
		Expect(manager.ViewBody).To(BeTrue())
		Expect(manager.Edit).To(BeFalse())
		Expect(manager.Delete).To(BeTrue())
		setter := contentdomain.EditorialPermissions(parent("setter"), &author, "public", "published", true, false)
		Expect(setter.Edit).To(BeFalse())
		Expect(setter.Delete).To(BeTrue())
		Expect(contentdomain.EditorialPermissions(parent("setter"), &author, "private", "draft", false, false).View).To(BeFalse())
	})
	It("requires practice completion before a reader can open, vote or discuss solved-only content", func() {
		locked := contentdomain.EditorialPermissions(parent("reader"), &author, "public", "published", true, false)
		Expect(locked.View).To(BeTrue())
		Expect(locked.ViewBody).To(BeFalse())
		Expect(locked.Comment).To(BeFalse())
		Expect(locked.Vote).To(BeFalse())
		unlocked := contentdomain.EditorialPermissions(parent("reader"), &author, "public", "published", true, true)
		Expect(unlocked.ViewBody && unlocked.Comment && unlocked.Vote).To(BeTrue())
	})
	It("does not equate ownership with permission to create new content", func() {
		p := parent(author)
		p.Scope.RolePermissions = nil
		value := contentdomain.EditorialPermissions(p, &author, "public", "published", false, false)
		Expect(value.Edit).To(BeTrue())
		Expect(value.Vote || value.Comment).To(BeFalse())
		p.Scope.Domain.Archived = true
		value = contentdomain.EditorialPermissions(p, &author, "public", "published", false, false)
		Expect(value.ViewBody).To(BeTrue())
		Expect(value.Edit || value.Delete).To(BeFalse())
	})
	It("keeps comment editing author-only even for moderators", func() {
		p := parent("manager")
		value := contentdomain.PostPermissions(p.Scope, true, true, &author)
		Expect(value.Edit).To(BeFalse())
		Expect(value.Delete).To(BeTrue())
		value = contentdomain.PostPermissions(parent(author).Scope, true, false, &author)
		Expect(value.Edit).To(BeTrue())
		Expect(contentdomain.PostPermissions(parent(author).Scope, false, true, &author)).To(Equal(contentdomain.Permissions{}))
	})
})
var _ = Describe("DiscussionPost.Edited", func() {
	It("distinguishes later edits from initial timestamps", func() {
		at := time.Now()
		post := contentdomain.DiscussionPost{CreatedAt: at, UpdatedAt: at}
		Expect(post.Edited()).To(BeFalse())
		post.UpdatedAt = at.Add(5 * time.Second)
		Expect(post.Edited()).To(BeTrue())
	})
})

type fakeScopeAccess struct{ visible bool }

func (f *fakeScopeAccess) CanViewProblem(context.Context, string, string) (bool, error) {
	return f.visible, nil
}

type fakeEditorialRepository struct {
	item    *contentdomain.Editorial
	input   contentdomain.EditorialInput
	filters contentdomain.EditorialFilters
}

func (f *fakeEditorialRepository) List(_ context.Context, filters contentdomain.EditorialFilters) ([]contentdomain.EditorialSummary, int, error) {
	f.filters = filters
	return nil, 0, nil
}
func (f *fakeEditorialRepository) Get(context.Context, string, string) (*contentdomain.Editorial, error) {
	return f.item, nil
}
func (f *fakeEditorialRepository) Create(_ context.Context, _ string, input contentdomain.EditorialInput) (*contentdomain.Editorial, error) {
	f.input = input
	return &contentdomain.Editorial{}, nil
}
func (f *fakeEditorialRepository) Update(_ context.Context, _, _ string, input contentdomain.EditorialInput) (*contentdomain.Editorial, error) {
	f.input = input
	return &contentdomain.Editorial{}, nil
}
func (f *fakeEditorialRepository) Delete(context.Context, string, string) error { return nil }
func (f *fakeEditorialRepository) Vote(context.Context, string, string, bool) (int, error) {
	return 0, nil
}

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Content service") }
