package application_test

import (
	"context"
	"errors"
	"testing"

	profileapp "github.com/RimuruChan/Vertex/server/internal/modules/profile/application"
	profiledomain "github.com/RimuruChan/Vertex/server/internal/modules/profile/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service", func() {
	DescribeTable("treats a blank username as missing without touching persistence",
		func(username string) {
			repository := &fakeProfileRepository{}
			service := profileapp.NewService(repository)
			_, err := service.ByUsername(context.Background(), username)
			Expect(err).To(MatchError(profiledomain.ErrNotFound))
			Expect(repository.queried).To(BeFalse())
		},
		Entry("empty", ""),
		Entry("whitespace", "   "),
	)

	It("trims the username before looking it up", func() {
		repository := &fakeProfileRepository{profile: &profiledomain.Profile{Username: "alice"}}
		service := profileapp.NewService(repository)
		result, err := service.ByUsername(context.Background(), "  alice  ")
		Expect(err).NotTo(HaveOccurred())
		Expect(repository.requested).To(Equal("alice"))
		Expect(result.Username).To(Equal("alice"))
	})

	It("propagates persistence failures unchanged", func() {
		failure := errors.New("database unavailable")
		service := profileapp.NewService(&fakeProfileRepository{err: failure})
		_, err := service.ByUsername(context.Background(), "alice")
		Expect(err).To(MatchError(failure))
	})
})

type fakeProfileRepository struct {
	profile   *profiledomain.Profile
	err       error
	requested string
	queried   bool
}

func (f *fakeProfileRepository) ByUsername(_ context.Context, username string) (*profiledomain.Profile, error) {
	f.queried = true
	f.requested = username
	if f.err != nil {
		return nil, f.err
	}
	return f.profile, nil
}

func TestService(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Profile service") }
