package content

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Editorial validation", func() {
	ctx := context.Background()

	It("trims and defaults the editable fields", func() {
		editorials := &fakeEditorialRepository{}
		service := newTestService(editorials, &fakeDiscussionRepository{})
		_, err := service.CreateEditorial(ctx, "user-1", false, EditorialInput{
			ProblemID: " problem-1 ", Title: " 题解 ", ContentMD: " body ",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(editorials.input.ProblemID).To(Equal("problem-1"))
		Expect(editorials.input.Title).To(Equal("题解"))
		Expect(editorials.input.ContentMD).To(Equal("body"))
		Expect(editorials.input.Visibility).To(Equal(VisibilityPublic))
		Expect(editorials.input.Status).To(Equal(StatusPublished))
	})

	It("requires a problem, title and body", func() {
		service := newTestService(&fakeEditorialRepository{}, &fakeDiscussionRepository{})
		for _, input := range []EditorialInput{
			{Title: "t", ContentMD: "b"},
			{ProblemID: "p", ContentMD: "b"},
			{ProblemID: "p", Title: "t"},
		} {
			_, err := service.CreateEditorial(ctx, "user-1", false, input)
			Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
		}
	})

	It("rejects an unknown visibility or status", func() {
		service := newTestService(&fakeEditorialRepository{}, &fakeDiscussionRepository{})
		_, err := service.CreateEditorial(ctx, "user-1", false, EditorialInput{
			ProblemID: "p", Title: "t", ContentMD: "b", Visibility: "secret",
		})

		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
		_, err = service.CreateEditorial(ctx, "user-1", false, EditorialInput{
			ProblemID: "p", Title: "t", ContentMD: "b", Status: "review",
		})

		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
	})

	It("keeps an editorial attached to the problem it was written for", func() {
		author := "user-1"
		editorials := &fakeEditorialRepository{
			item: &Editorial{ID: "e1", ProblemID: "problem-1", AuthorID: &author, Status: StatusPublished},
		}
		service := newTestService(editorials, &fakeDiscussionRepository{})
		_, err := service.UpdateEditorial(ctx, "e1", author, false, EditorialInput{
			ProblemID: "problem-2", Title: "t", ContentMD: "b",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(editorials.input.ProblemID).To(Equal("problem-1"))
	})
})

var _ = Describe("Editorial access", func() {
	ctx := context.Background()
	author := "user-1"

	published := func() *Editorial {
		return &Editorial{
			ID: "e1", ProblemID: "problem-1", AuthorID: &author,
			ContentMD: "答案是 42", Status: StatusPublished, Visibility: VisibilityPublic,
		}
	}

	It("hides a draft from everyone but its author", func() {
		editorials := &fakeEditorialRepository{item: published()}
		editorials.item.Status = StatusDraft
		service := newTestService(editorials, &fakeDiscussionRepository{})

		_, err := service.GetEditorial(ctx, "e1", "user-2", false)
		Expect(err).To(MatchError(ErrNotFound))

		item, err := service.GetEditorial(ctx, "e1", author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.ContentMD).NotTo(BeEmpty())
	})

	It("hides an editorial when its problem is not visible", func() {
		editorials := &fakeEditorialRepository{item: published()}
		service := NewService(editorials, &fakeDiscussionRepository{}, &fakeScopeAccess{})

		_, err := service.GetEditorial(ctx, "e1", "user-2", false)
		Expect(err).To(MatchError(ErrNotFound))

		// The editorial author can still manage work they already wrote if the
		// problem is unpublished later.
		item, err := service.GetEditorial(ctx, "e1", author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.ID).To(Equal("e1"))
	})

	It("does not create an editorial for an inaccessible problem", func() {
		editorials := &fakeEditorialRepository{}
		service := NewService(editorials, &fakeDiscussionRepository{}, &fakeScopeAccess{})

		_, err := service.CreateEditorial(ctx, "user-1", false, EditorialInput{
			ProblemID: "problem-1", Title: "题解", ContentMD: "答案",
		})
		Expect(err).To(MatchError(ErrNotFound))
		Expect(editorials.input).To(Equal(EditorialInput{}))
	})

	It("keeps solved-only and vote state on the body-free list summary", func() {
		editorials := &fakeEditorialRepository{summary: &EditorialSummary{
			ID: "e1", ProblemID: "problem-1", Title: "题解",
			Status: StatusPublished, Visibility: VisibilityPublic,
			SolvedOnly: true, Voted: true, VoteCount: 3,
		}}
		service := newTestService(editorials, &fakeDiscussionRepository{})

		items, total, err := service.ListEditorials(ctx, EditorialFilters{ViewerID: "user-2"})
		Expect(err).NotTo(HaveOccurred())
		Expect(total).To(Equal(1))
		Expect(items).To(HaveLen(1))
		Expect(items[0].Locked).To(BeTrue())
		Expect(items[0].Voted).To(BeTrue())
		Expect(items[0].VoteCount).To(Equal(3))
	})

	It("withholds a solved-only body from a reader who has not solved it", func() {
		editorials := &fakeEditorialRepository{item: published(), solved: false}
		editorials.item.SolvedOnly = true
		service := newTestService(editorials, &fakeDiscussionRepository{})

		item, err := service.GetEditorial(ctx, "e1", "user-2", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Locked).To(BeTrue())
		Expect(item.ContentMD).To(BeEmpty())
		// The entry itself stays visible so the reader knows it exists.
		Expect(item.Title).To(Equal(item.Title))
	})

	It("unlocks a solved-only body once the reader solved the problem", func() {
		editorials := &fakeEditorialRepository{item: published(), solved: true}
		editorials.item.SolvedOnly = true
		service := newTestService(editorials, &fakeDiscussionRepository{})

		item, err := service.GetEditorial(ctx, "e1", "user-2", false)
		Expect(err).NotTo(HaveOccurred())
		Expect(item.Locked).To(BeFalse())
		Expect(item.ContentMD).To(Equal("答案是 42"))
	})

	It("never locks the author or an administrator out of the body", func() {
		editorials := &fakeEditorialRepository{item: published(), solved: false}
		editorials.item.SolvedOnly = true
		service := newTestService(editorials, &fakeDiscussionRepository{})

		own, err := service.GetEditorial(ctx, "e1", author, false)
		Expect(err).NotTo(HaveOccurred())
		Expect(own.Locked).To(BeFalse())

		editorials.item = published()
		editorials.item.SolvedOnly = true
		moderator, err := service.GetEditorial(ctx, "e1", "admin-1", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(moderator.Locked).To(BeFalse())
	})

	It("refuses edits and deletes from other users", func() {
		editorials := &fakeEditorialRepository{item: published()}
		service := newTestService(editorials, &fakeDiscussionRepository{})

		_, err := service.UpdateEditorial(ctx, "e1", "user-2", false, EditorialInput{Title: "t", ContentMD: "b"})
		Expect(err).To(MatchError(ErrForbidden))
		Expect(service.DeleteEditorial(ctx, "e1", "user-2", false)).To(MatchError(ErrForbidden))
	})

	It("lets an administrator remove any editorial as moderation", func() {
		editorials := &fakeEditorialRepository{item: published()}
		service := newTestService(editorials, &fakeDiscussionRepository{})
		Expect(service.DeleteEditorial(ctx, "e1", "admin-1", true)).To(Succeed())
		Expect(editorials.deleted).To(BeTrue())
	})

	It("requires a signed-in user to vote", func() {
		editorials := &fakeEditorialRepository{item: published()}
		service := newTestService(editorials, &fakeDiscussionRepository{})
		_, err := service.VoteEditorial(ctx, "e1", "", false, true)
		Expect(err).To(MatchError(ErrForbidden))
	})

	It("does not accept votes for an unpublished editorial", func() {
		editorials := &fakeEditorialRepository{item: published()}
		editorials.item.Status = StatusDraft
		service := newTestService(editorials, &fakeDiscussionRepository{})
		_, err := service.VoteEditorial(ctx, "e1", "user-2", false, true)
		Expect(err).To(MatchError(ErrNotFound))
	})
})

var _ = Describe("Discussion posts", func() {
	ctx := context.Background()

	It("validates parent identifiers", func() {
		service := newTestService(&fakeEditorialRepository{}, &fakeDiscussionRepository{})
		parentID := int64(0)
		_, err := service.CreateProblemPost(ctx, "problem-1", "user-1", false, "body", &parentID)
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
	})

	It("does not read or create discussions on an inaccessible problem", func() {
		repository := &fakeDiscussionRepository{}
		service := NewService(&fakeEditorialRepository{}, repository, &fakeScopeAccess{})

		posts, err := service.ListProblemPosts(ctx, "problem-1", "user-1", false)
		Expect(err).To(MatchError(ErrNotFound))
		Expect(posts).To(BeNil())
		_, err = service.CreateProblemPost(ctx, "problem-1", "user-1", false, "body", nil)
		Expect(err).To(MatchError(ErrNotFound))
	})

	It("does not expose an editorial discussion when its problem is inaccessible", func() {
		author := "author-1"
		editorials := &fakeEditorialRepository{item: &Editorial{
			ID: "e1", ProblemID: "problem-1", AuthorID: &author,
			Status: StatusPublished, Visibility: VisibilityPublic,
		}}
		service := NewService(editorials, &fakeDiscussionRepository{}, &fakeScopeAccess{})

		posts, err := service.ListEditorialPosts(ctx, "e1", "user-1", false)
		Expect(err).To(MatchError(ErrNotFound))
		Expect(posts).To(BeNil())
	})

	It("does not let editorial discussions bypass the solved-only gate", func() {
		author := "author-1"
		editorials := &fakeEditorialRepository{item: &Editorial{
			ID: "e1", ProblemID: "problem-1", AuthorID: &author,
			Status: StatusPublished, Visibility: VisibilityPublic,
			SolvedOnly: true, ContentMD: "spoiler",
		}}
		service := newTestService(editorials, &fakeDiscussionRepository{})

		posts, err := service.ListEditorialPosts(ctx, "e1", "user-1", false)
		Expect(err).To(MatchError(ErrSpoilerLocked))
		Expect(posts).To(BeNil())
		_, err = service.CreateEditorialPost(ctx, "e1", "user-1", false, "tell me")
		Expect(err).To(MatchError(ErrSpoilerLocked))
	})

	It("does not read or create discussions on an inaccessible contest", func() {
		service := NewService(
			&fakeEditorialRepository{}, &fakeDiscussionRepository{},
			&fakeScopeAccess{problem: true},
		)

		posts, err := service.ListContestPosts(ctx, "contest-1", "user-1", false)
		Expect(err).To(MatchError(ErrNotFound))
		Expect(posts).To(BeNil())
		_, err = service.CreateContestPost(ctx, "contest-1", "user-1", false, "body", nil)
		Expect(err).To(MatchError(ErrNotFound))
	})

	It("requires replies to use a parent from the same discussion", func() {
		otherProblemID := "problem-2"
		repository := &fakeDiscussionRepository{
			post: &DiscussionPost{ID: 1, ProblemID: &otherProblemID},
		}
		service := newTestService(&fakeEditorialRepository{}, repository)
		parentID := int64(1)

		_, err := service.CreateProblemPost(ctx, "problem-1", "user-1", false, "body", &parentID)
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("different discussion")))
	})

	It("accepts a reply whose parent belongs to the same discussion", func() {
		problemID := "problem-1"
		repository := &fakeDiscussionRepository{
			post: &DiscussionPost{ID: 1, ProblemID: &problemID},
		}
		service := newTestService(&fakeEditorialRepository{}, repository)
		parentID := int64(1)

		_, err := service.CreateProblemPost(ctx, problemID, "user-1", false, "body", &parentID)
		Expect(err).NotTo(HaveOccurred())
	})

	It("applies the same parent-scope rule to contest discussions", func() {
		contestID := "contest-1"
		repository := &fakeDiscussionRepository{
			post: &DiscussionPost{ID: 1, ContestID: &contestID},
		}
		service := newTestService(&fakeEditorialRepository{}, repository)
		parentID := int64(1)

		_, err := service.CreateContestPost(ctx, "contest-2", "user-1", false, "body", &parentID)
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
		_, err = service.CreateContestPost(ctx, contestID, "user-1", false, "body", &parentID)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects an empty or oversized body", func() {
		service := newTestService(&fakeEditorialRepository{}, &fakeDiscussionRepository{})
		_, err := service.CreateProblemPost(ctx, "problem-1", "user-1", false, "   ", nil)
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())

		long := make([]byte, maxPostContent+1)
		for i := range long {
			long[i] = 'a'
		}
		_, err = service.CreateProblemPost(ctx, "problem-1", "user-1", false, string(long), nil)
		Expect(errors.Is(err, ErrInvalidInput)).To(BeTrue())
	})

	It("allows owners and administrators to delete posts", func() {
		repository := &fakeDiscussionRepository{owner: false}
		service := newTestService(&fakeEditorialRepository{}, repository)
		Expect(service.DeletePost(ctx, 1, "user-1", false)).To(MatchError(ErrForbidden))
		Expect(repository.deleted).To(BeFalse())

		Expect(service.DeletePost(ctx, 1, "admin-1", true)).To(Succeed())
		Expect(repository.deleted).To(BeTrue())
	})

	It("lets only the author edit a post, administrators included", func() {
		author := "user-1"
		repository := &fakeDiscussionRepository{post: &DiscussionPost{ID: 1, AuthorID: &author}}
		service := newTestService(&fakeEditorialRepository{}, repository)

		_, err := service.UpdatePost(ctx, 1, "user-2", "new text")
		Expect(err).To(MatchError(ErrForbidden))
		// Moderation removes posts; it does not rewrite them under someone
		// else's name.
		_, err = service.UpdatePost(ctx, 1, "admin-1", "new text")
		Expect(err).To(MatchError(ErrForbidden))

		post, err := service.UpdatePost(ctx, 1, author, "new text")
		Expect(err).NotTo(HaveOccurred())
		Expect(post.ContentMD).To(Equal("new text"))
	})
})

var _ = Describe("DiscussionPost.Edited", func() {
	It("only reports posts changed well after they were written", func() {
		created := time.Now()
		post := DiscussionPost{CreatedAt: created, UpdatedAt: created}
		Expect(post.Edited()).To(BeFalse())

		post.UpdatedAt = created.Add(5 * time.Second)
		Expect(post.Edited()).To(BeTrue())
	})
})

// ---------- fakes ----------

func newTestService(editorials EditorialRepository, discussions DiscussionRepository) *Service {
	return NewService(editorials, discussions, &fakeScopeAccess{problem: true, contest: true})
}

type fakeScopeAccess struct {
	problem bool
	contest bool
	err     error
}

func (f *fakeScopeAccess) CanViewProblem(context.Context, string, string, bool) (bool, error) {
	return f.problem, f.err
}

func (f *fakeScopeAccess) CanViewContest(context.Context, string, string, bool) (bool, error) {
	return f.contest, f.err
}

type fakeEditorialRepository struct {
	item    *Editorial
	summary *EditorialSummary
	input   EditorialInput
	solved  bool
	deleted bool
	votes   int
}

func (f *fakeEditorialRepository) List(context.Context, EditorialFilters) ([]EditorialSummary, int, error) {
	if f.summary != nil {
		return []EditorialSummary{*f.summary}, 1, nil
	}
	if f.item == nil {
		return nil, 0, nil
	}
	return []EditorialSummary{{
		ID: f.item.ID, ProblemID: f.item.ProblemID, ProblemTitle: f.item.ProblemTitle,
		AuthorID: f.item.AuthorID, AuthorName: f.item.AuthorName, Title: f.item.Title,
		Visibility: f.item.Visibility, Status: f.item.Status,
		SolvedOnly: f.item.SolvedOnly, VoteCount: f.item.VoteCount, Voted: f.item.Voted,
		Locked: f.item.Locked, CreatedAt: f.item.CreatedAt, UpdatedAt: f.item.UpdatedAt,
	}}, 1, nil
}

func (f *fakeEditorialRepository) Get(context.Context, string, string) (*Editorial, error) {
	if f.item == nil {
		return nil, ErrNotFound
	}
	copied := *f.item
	return &copied, nil
}

func (f *fakeEditorialRepository) Create(_ context.Context, _ string, input EditorialInput) (*Editorial, error) {
	f.input = input
	return &Editorial{ID: "editorial-1"}, nil
}

func (f *fakeEditorialRepository) Update(_ context.Context, _ string, input EditorialInput) (*Editorial, error) {
	f.input = input
	return &Editorial{ID: "editorial-1"}, nil
}

func (f *fakeEditorialRepository) Delete(context.Context, string) error {
	f.deleted = true
	return nil
}

func (f *fakeEditorialRepository) Vote(context.Context, string, string, bool) (int, error) {
	f.votes++
	return f.votes, nil
}

func (f *fakeEditorialRepository) HasSolved(context.Context, string, string) (bool, error) {
	return f.solved, nil
}

type fakeDiscussionRepository struct {
	owner   bool
	deleted bool
	post    *DiscussionPost
}

func (f *fakeDiscussionRepository) ListByProblem(context.Context, string) ([]DiscussionPost, error) {
	return nil, nil
}

func (f *fakeDiscussionRepository) ListByEditorial(context.Context, string) ([]DiscussionPost, error) {
	return nil, nil
}

func (f *fakeDiscussionRepository) ListByContest(context.Context, string) ([]DiscussionPost, error) {
	return nil, nil
}

func (f *fakeDiscussionRepository) Get(context.Context, int64) (*DiscussionPost, error) {
	if f.post == nil {
		return nil, ErrNotFound
	}
	return f.post, nil
}

func (f *fakeDiscussionRepository) CreateProblemPost(context.Context, string, string, string, *int64) (*DiscussionPost, error) {
	return &DiscussionPost{ID: 1}, nil
}

func (f *fakeDiscussionRepository) CreateEditorialPost(context.Context, string, string, string) (*DiscussionPost, error) {
	return &DiscussionPost{ID: 1}, nil
}

func (f *fakeDiscussionRepository) CreateContestPost(context.Context, string, string, string, *int64) (*DiscussionPost, error) {
	return &DiscussionPost{ID: 1}, nil
}

func (f *fakeDiscussionRepository) Update(_ context.Context, _ int64, contentMD string) (*DiscussionPost, error) {
	return &DiscussionPost{ID: 1, ContentMD: contentMD}, nil
}

func (f *fakeDiscussionRepository) IsPostOwner(context.Context, int64, string) (bool, error) {
	return f.owner, nil
}

func (f *fakeDiscussionRepository) Delete(context.Context, int64) error {
	f.deleted = true
	return nil
}
