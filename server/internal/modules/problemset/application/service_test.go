package application_test

import (
	"context"
	"strings"
	"testing"

	setapp "github.com/RimuruChan/Vertex/server/internal/modules/problemset/application"
	setdomain "github.com/RimuruChan/Vertex/server/internal/modules/problemset/domain"
	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fakeRepository struct {
	set      *setdomain.Set
	filters  setdomain.Filters
	upserted *setdomain.UpsertInput
	items    []setdomain.ItemInput
	deleted  string
}

func (r *fakeRepository) List(_ context.Context, filters setdomain.Filters) ([]setdomain.Set, int, error) {
	r.filters = filters
	return []setdomain.Set{*r.set}, 1, nil
}

func (r *fakeRepository) Get(context.Context, string, string) (*setdomain.Set, error) {
	if r.set == nil {
		return nil, setdomain.ErrNotFound
	}
	copied := *r.set
	copied.Items = append([]setdomain.Item(nil), r.set.Items...)
	return &copied, nil
}

func (r *fakeRepository) Create(_ context.Context, _ string, input setdomain.UpsertInput) (*setdomain.Set, error) {
	r.upserted = &input
	return r.set, nil
}

func (r *fakeRepository) Update(_ context.Context, _ string, _ string, input setdomain.UpsertInput) (*setdomain.Set, error) {
	r.upserted = &input
	return r.set, nil
}

func (r *fakeRepository) Delete(_ context.Context, id, _ string) error {
	r.deleted = id
	return nil
}

func (r *fakeRepository) SetItems(_ context.Context, _ string, _ string, items []setdomain.ItemInput) error {
	r.items = items
	return nil
}

func owned(userID string) *setdomain.Set {
	return &setdomain.Set{
		ID: "set-1", Title: "入门 DP", AuthorID: &userID,
		Visibility: setdomain.VisibilityPublic,
	}
}

var _ = Describe("Problem set permissions", func() {
	It("keeps ownership separate from creation capability and editors separate from publication", func() {
		scope := tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public"}, UserID: "owner", MemberStatus: "active"}
		owner := setdomain.EffectivePermissions(scope, "owner", "private", "")
		Expect(owner.Edit).To(BeTrue())
		Expect(owner.Transfer).To(BeTrue())
		scope.UserID = "editor"
		editor := setdomain.EffectivePermissions(scope, "owner", "private", setdomain.AccessEditor)
		Expect(editor.View).To(BeTrue())
		Expect(editor.Edit).To(BeTrue())
		Expect(editor.Publish).To(BeFalse())
		Expect(editor.Delete).To(BeFalse())
		reader := setdomain.EffectivePermissions(scope, "owner", "private", setdomain.AccessReader)
		Expect(reader.View).To(BeTrue())
		Expect(reader.Edit).To(BeFalse())
	})
	It("makes archived domains read-only and suspended owners inaccessible", func() {
		scope := tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public", Archived: true}, UserID: "owner", MemberStatus: "active"}
		value := setdomain.EffectivePermissions(scope, "owner", "private", "")
		Expect(value.View).To(BeTrue())
		Expect(value.Edit).To(BeFalse())
		Expect(value.Transfer).To(BeFalse())
		scope.MemberStatus = "suspended"
		Expect(setdomain.EffectivePermissions(scope, "owner", "public", "")).To(Equal(setdomain.Permissions{}))
	})
	It("does not turn an unjoined user or a group manager into a resource collaborator", func() {
		scope := tenancydomain.Scope{Domain: tenancydomain.Domain{Visibility: "public"}, UserID: "reader"}
		Expect(setdomain.EffectivePermissions(scope, "owner", "private", setdomain.AccessReader).View).To(BeFalse())
		scope.MemberStatus = "active"
		scope.RolePermissions = []tenancydomain.Permission{tenancydomain.ManageGroups}
		Expect(setdomain.EffectivePermissions(scope, "owner", "private", "").View).To(BeFalse())
		scope.RolePermissions = []tenancydomain.Permission{tenancydomain.ManageResources}
		Expect(setdomain.EffectivePermissions(scope, "owner", "private", "").ManageAccess).To(BeTrue())
	})
})

var _ = Describe("Problem set validation", func() {
	ctx := context.Background()

	It("requires a title", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setdomain.UpsertInput{Title: "  "})
		Expect(err).To(MatchError(setdomain.ErrInvalidInput))
	})

	It("defaults visibility to public", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		_, err := service.Create(ctx, "user-1", setdomain.UpsertInput{Title: "题单"})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.upserted.Visibility).To(Equal(setdomain.VisibilityPublic))
	})

	It("rejects an unknown visibility", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setdomain.UpsertInput{Title: "t", Visibility: "secret"})
		Expect(err).To(MatchError(setdomain.ErrInvalidInput))
	})

	It("rejects an over-long title", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		_, err := service.Create(ctx, "user-1", setdomain.UpsertInput{Title: strings.Repeat("a", 200)})
		Expect(err).To(MatchError(setdomain.ErrInvalidInput))
	})

	It("rejects the same problem twice in one set", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		err := service.SetItems(ctx, "set-1", "user-1", []setdomain.ItemInput{
			{ProblemID: "p1"}, {ProblemID: "p1"},
		})
		Expect(err).To(MatchError(setdomain.ErrInvalidInput))
	})

	It("keeps the curator's order and trims notes", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		Expect(service.SetItems(ctx, "set-1", "user-1", []setdomain.ItemInput{
			{ProblemID: "p2", Note: "  先做这题  "},
			{ProblemID: "p1"},
		})).To(Succeed())
		Expect(repository.items[0].ProblemID).To(Equal("p2"))
		Expect(repository.items[0].Note).To(Equal("先做这题"))
	})

	It("rejects a set larger than the curation limit", func() {
		service := setapp.NewService(&fakeRepository{set: owned("user-1")})
		items := make([]setdomain.ItemInput, 501)
		for i := range items {
			items[i] = setdomain.ItemInput{ProblemID: string(rune('a' + i%26))}
		}
		err := service.SetItems(ctx, "set-1", "user-1", items)
		Expect(err).To(MatchError(setdomain.ErrInvalidInput))
	})
})

var _ = Describe("Problem set listing", func() {
	It("clamps the page size", func() {
		repository := &fakeRepository{set: owned("user-1")}
		service := setapp.NewService(repository)
		_, _, err := service.List(context.Background(), setdomain.Filters{Limit: 5000})
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.filters.Limit).To(Equal(20))
	})
})

func (r *fakeRepository) Grants(context.Context, string) ([]setdomain.AccessGrant, error) {
	return nil, nil
}

func (r *fakeRepository) SetGrant(context.Context, string, setdomain.GrantInput) error { return nil }

func (r *fakeRepository) RemoveGrant(context.Context, string, int64) error { return nil }

func (r *fakeRepository) Transfer(context.Context, string, string) error { return nil }

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Problem set service") }
