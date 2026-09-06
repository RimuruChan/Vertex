package contest_test

import (
	"context"
	"time"

	contestapp "github.com/RimuruChan/Vertex/server/internal/contest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type problemAccessCase struct {
	visibility  string
	participant bool
	staff       string
	creator     bool
	role        string
	beforeStart bool
	ended       bool
	wantErr     error
}

var _ = Describe("Contest problem access", func() {
	DescribeTable("keeps unpublished statements contest-scoped",
		func(test problemAccessCase) {
			const userID = "user-1"
			createdBy := "admin-1"
			if test.creator {
				createdBy = userID
			}
			begin := time.Now().Add(-time.Hour)
			end := time.Now().Add(time.Hour)
			if test.beforeStart {
				begin = time.Now().Add(time.Hour)
				end = begin.Add(time.Hour)
			}
			if test.ended {
				begin = time.Now().Add(-2 * time.Hour)
				end = time.Now().Add(-time.Hour)
			}
			repository := &fakeRepository{
				admin: test.role == "admin",
				contest: &contestapp.Contest{
					ID: "contest-1", Visibility: test.visibility, CreatedBy: &createdBy,
					BeginAt: begin, EndAt: end,
				},
				participant: test.participant, staffRole: test.staff,
				problemDetail: &contestapp.ProblemDetail{
					Problem: contestapp.Problem{
						ContestID: "contest-1", ProblemID: "problem-1",
						Title: "Hidden", Visibility: "draft",
					},
					StatementMD: "secret statement",
				},
			}
			service := contestapp.NewService(repository, nil)

			item, err := service.Problem(context.Background(),
				"contest-1", "problem-1", userID, test.role)
			if test.wantErr != nil {
				Expect(err).To(MatchError(test.wantErr))
				Expect(item).To(BeNil())
				Expect(repository.problemReads).To(BeZero())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(item.StatementMD).To(Equal("secret statement"))
			Expect(repository.problemReads).To(Equal(1))
		},
		Entry("allows a registered public-contest participant", problemAccessCase{
			visibility: "public", participant: true,
		}),
		Entry("rejects a public-contest outsider", problemAccessCase{
			visibility: "public", wantErr: contestapp.ErrRegistrationNeeded,
		}),
		Entry("allows a registered password-contest participant", problemAccessCase{
			visibility: "password", participant: true,
		}),
		Entry("rejects a password-contest outsider", problemAccessCase{
			visibility: "password", wantErr: contestapp.ErrRegistrationNeeded,
		}),
		Entry("hides a private contest from an outsider", problemAccessCase{
			visibility: "private", wantErr: contestapp.ErrNotFound,
		}),
		Entry("allows private-contest jury", problemAccessCase{
			visibility: "private", staff: contestapp.StaffJury,
		}),
		Entry("allows private-contest observers", problemAccessCase{
			visibility: "private", staff: contestapp.StaffObserver,
		}),
		Entry("allows the private-contest creator", problemAccessCase{
			visibility: "private", creator: true,
		}),
		Entry("allows a global administrator", problemAccessCase{
			visibility: "private", role: "admin",
		}),
		Entry("hides a pre-start problem from a participant", problemAccessCase{
			visibility: "public", participant: true, beforeStart: true,
			wantErr: contestapp.ErrNotFound,
		}),
		Entry("keeps the statement available to a participant after the contest", problemAccessCase{
			visibility: "public", participant: true, ended: true,
		}),
	)

	It("hides a problem that is not linked to the contest", func() {
		repository := &fakeRepository{
			contest: &contestapp.Contest{
				ID: "contest-1", Visibility: "public",
				BeginAt: time.Now().Add(-time.Hour), EndAt: time.Now().Add(time.Hour),
			},
			participant: true, problemErr: contestapp.ErrProblemNotInContest,
		}
		service := contestapp.NewService(repository, nil)
		_, err := service.Problem(context.Background(),
			"contest-1", "other-problem", "user-1", "user")
		Expect(err).To(MatchError(contestapp.ErrNotFound))
	})
})
