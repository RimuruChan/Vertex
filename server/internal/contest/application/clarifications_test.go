package application_test

import (
	"context"
	"errors"

	contestapp "github.com/RimuruChan/Vertex/server/internal/contest/application"
	contestdomain "github.com/RimuruChan/Vertex/server/internal/contest/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type clarificationAccessRepository struct {
	*fakeRepository
	listCalls int
	viewer    contestdomain.Viewer
}

func (r *clarificationAccessRepository) CreateClarification(
	context.Context, contestdomain.ClarificationInput,
) (*contestdomain.Clarification, error) {
	return nil, nil
}

func (r *clarificationAccessRepository) ListClarifications(
	_ context.Context, _ string, viewer contestdomain.Viewer,
) ([]contestdomain.Clarification, error) {
	r.listCalls++
	r.viewer = viewer
	return []contestdomain.Clarification{{ID: 1, FromJury: true}}, nil
}

func (r *clarificationAccessRepository) GetClarification(
	context.Context, string, int64,
) (*contestdomain.Clarification, error) {
	return nil, contestdomain.ErrClarificationNotFound
}

type clarificationAccessCase struct {
	visibility  string
	role        string
	staff       string
	participant bool
	creator     bool
	wantErr     error
	wantFull    bool
}

var _ = Describe("Clarification access", func() {
	DescribeTable("fails closed before loading messages",
		func(test clarificationAccessCase) {
			const userID = "user-1"
			createdBy := "admin-1"
			if test.creator {
				createdBy = userID
			}
			base := &fakeRepository{
				admin: test.role == "admin",
				contest: &contestdomain.Contest{
					ID: "contest-1", Visibility: test.visibility, CreatedBy: &createdBy,
				},
				staffRole: test.staff, participant: test.participant,
			}
			repository := &clarificationAccessRepository{fakeRepository: base}
			service := contestapp.NewService(repository, nil)

			items, err := service.Clarifications(context.Background(), "contest-1", userID, test.role)
			if test.wantErr != nil {
				Expect(err).To(MatchError(test.wantErr))
				Expect(items).To(BeNil())
				Expect(repository.listCalls).To(Equal(0))
				return
			}

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1))
			Expect(repository.listCalls).To(Equal(1))
			Expect(repository.viewer.IsStaff()).To(Equal(test.wantFull))
		},
		Entry("allows a public contest reader", clarificationAccessCase{visibility: "public"}),
		Entry("rejects an unregistered password-contest reader", clarificationAccessCase{
			visibility: "password", wantErr: contestdomain.ErrRegistrationNeeded,
		}),
		Entry("allows a password-contest participant", clarificationAccessCase{
			visibility: "password", participant: true,
		}),
		Entry("hides a private contest from an outsider", clarificationAccessCase{
			visibility: "private", wantErr: contestdomain.ErrNotFound,
		}),
		Entry("allows private-contest jury", clarificationAccessCase{
			visibility: "private", staff: contestdomain.StaffJury, wantFull: true,
		}),
		Entry("allows private-contest observers", clarificationAccessCase{
			visibility: "private", staff: contestdomain.StaffObserver, wantFull: true,
		}),
		Entry("allows the private-contest creator a full read view", clarificationAccessCase{
			visibility: "private", creator: true, wantFull: true,
		}),
		Entry("allows global administrators", clarificationAccessCase{
			visibility: "private", role: "admin", wantFull: true,
		}),
		Entry("rejects an unknown persisted visibility", clarificationAccessCase{
			visibility: "unexpected", wantErr: contestdomain.ErrNotFound,
		}),
	)

	It("propagates participant lookup failures without loading messages", func() {
		base := &fakeRepository{contest: &contestdomain.Contest{ID: "contest-1", Visibility: "password"}}
		repository := &failingParticipantClarificationRepository{
			clarificationAccessRepository: &clarificationAccessRepository{fakeRepository: base},
		}
		service := contestapp.NewService(repository, nil)

		_, err := service.Clarifications(context.Background(), "contest-1", "user-1", "user")
		Expect(err).To(MatchError("participant lookup failed"))
		Expect(repository.listCalls).To(Equal(0))
	})
})

type failingParticipantClarificationRepository struct {
	*clarificationAccessRepository
}

func (r *failingParticipantClarificationRepository) IsParticipant(
	context.Context, string, string,
) (bool, error) {
	return false, errors.New("participant lookup failed")
}
