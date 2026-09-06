package problemset_test

import (
	"context"
	"strings"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	setapp "github.com/RimuruChan/Vertex/server/internal/problemset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeRepository struct {
	set      *setapp.Set
	filters  setapp.Filters
	upserted *setapp.UpsertInput
	items    []setapp.ItemInput
	deleted  string
}

func (r *fakeRepository) List(_ context.Context, filters setapp.Filters) ([]setapp.Set, int, error) {
	r.filters = filters
	return []setapp.Set{*r.set}, 1, nil
}

func (r *fakeRepository) Get(context.Context, string, string, bool) (*setapp.Set, error) {
	if r.set == nil {
		return nil, setapp.ErrNotFound
	}
	copied := *r.set
	copied.Items = append([]setapp.Item(nil), r.set.Items...)
	return &copied, nil
}

func (r *fakeRepository) Create(_ context.Context, _ string, input setapp.UpsertInput) (*setapp.Set, error) {
	r.upserted = &input
	return r.set, nil
}

func (r *fakeRepository) Update(_ context.Context, _, _ string, _ bool, input setapp.UpsertInput) (*setapp.Set, error) {
	r.upserted = &input
	return r.set, nil
}

func (r *fakeRepository) Delete(_ context.Context, id, _ string) error {
	r.deleted = id
	return nil
}

func (r *fakeRepository) SetItems(_ context.Context, _, _ string, _ bool, items []setapp.ItemInput) error {
	r.items = items
	return nil
}

func owned(userID string) *setapp.Set {
	return &setapp.Set{
		ID: "set-1", Title: "入门 DP", AuthorID: &userID,
		Visibility: setapp.VisibilityPublic,
	}
}

var _ = Describe("Problem set permissions", func() {
	It("keeps ownership separate from creation capability and editors separate from publication", func() {
		scope := domain.Scope{Domain: domain.Domain{Visibility: "public"}, UserID: "owner", MemberStatus: "active"}
		owner := setapp.EffectivePermissions(scope, "owner", "private", "")
		Expect(owner.Edit).To(BeTrue())
		Expect(owner.Transfer).To(BeTrue())
		scope.UserID = "editor"
		editor := setapp.EffectivePermissions(scope, "owner", "private", setapp.AccessEditor)
		Expect(editor.View).To(BeTrue())
		Expect(editor.Edit).To(BeTrue())
		Expect(editor.Publish).To(BeFalse())
		Expect(editor.Delete).To(BeFalse())
		reader := setapp.EffectivePermissions(scope, "owner", "private", setapp.AccessReader)
		Expect(reader.View).To(BeTrue())
		Expect(reader.Edit).To(BeFalse())
	})
	It("makes archived domains read-only and suspended owners inaccessible", func() {
		scope := domain.Scope{Domain: domain.Domain{Visibility: "public", Archived: true}, UserID: "owner", MemberStatus: "active"}
		value := setapp.EffectivePermissions(scope, "owner", "private", "")
		Expect(value.View).To(BeTrue())
		Expect(value.Edit).To(BeFalse())
		Expect(value.Transfer).To(BeFalse())
		scope.MemberStatus = "suspended"
		Expect(setapp.EffectivePermissions(scope, "owner", "public", "")).To(Equal(setapp.Permissions{}))
	})
	It("does not turn an unjoined user or a group manager into a resource collaborator", func() {
		scope := domain.Scope{Domain: domain.Domain{Visibility: "public"}, UserID: "reader"}
		Expect(setapp.EffectivePermissions(scope, "owner", "private", setapp.AccessReader).View).To(BeFalse())
		scope.MemberStatus = "active"
		scope.RolePermissions = []domain.Permission{domain.ManageGroups}
		Expect(setapp.EffectivePermissions(scope, "owner", "private", "").View).To(BeFalse())
		scope.RolePermissions = []domain.Permission{domain.ManageResources}
		Expect(setapp.EffectivePermissions(scope, "owner", "private", "").ManageAccess).To(BeTrue())
	})
})

var _ = Describe("Problem set validation", func() {
	ctx := context.Background()

	It("requires a title", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: "  "})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("defaults visibility to public", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: "题单"})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.upserted.Visibility).To(Equal(setapp.VisibilityPublic))
	})

	It("rejects an unknown visibility", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: "t", Visibility: "secret"})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("rejects an over-long title", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setapp.UpsertInput{Title: strings.Repeat("a", 200)})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("rejects the same problem twice in one set", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		err := service.SetItems(ctx, "set-1", "user-1", false, []setapp.ItemInput{
			{ProblemID: "p1"}, {ProblemID: "p1"},
		})
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})

	It("keeps the curator's order and trims notes", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		Expect(service.SetItems(ctx, "set-1", "user-1", false, []setapp.ItemInput{
			{ProblemID: "p2", Note: "  先做这题  "},
			{ProblemID: "p1"},
		})).To(Succeed())
		Expect(repository.items[0].ProblemID).To(Equal("p2"))
		Expect(repository.items[0].Note).To(Equal("先做这题"))
	})

	It("rejects a set larger than the curation limit", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		items := make([]setapp.ItemInput, 501)
		for i := range items {
			items[i] = setapp.ItemInput{ProblemID: string(rune('a' + i%26))}
		}
		err := service.SetItems(ctx, "set-1", "user-1", false, items)
		Expect(err).To(MatchError(setapp.ErrInvalidInput))
	})
})

var _ = Describe("Problem set listing", func() {
	It("clamps the page size", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		_, _, err := service.List(context.Background(), setapp.Filters{Limit: 5000})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Limit).To(Equal(20))
	})
})

func (r *fakeRepository) Grants(context.Context, string) ([]setapp.AccessGrant, error) {
	return nil, nil
}

func (r *fakeRepository) SetGrant(context.Context, string, setapp.GrantInput) error { return nil }

func (r *fakeRepository) RemoveGrant(context.Context, string, int64) error { return nil }

func (r *fakeRepository) Transfer(context.Context, string, string) error { return nil }
