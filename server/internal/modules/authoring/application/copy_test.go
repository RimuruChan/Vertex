package application

import (
	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
)

var _ = Describe("Copy input rules", func() {
	It("normalizes references and rejects missing release or attribution before persistence", func(ctx SpecContext) {
		packages := &fakePackages{}
		service := newService(packages, &fakeBuilds{})
		_, err := service.Copy(ctx, authoringdomain.CopyInput{SourceDomain: " source ", SourceProblem: "001000", SourceVersion: 1, Attribution: " Approved source "})
		Expect(err).NotTo(HaveOccurred())
		Expect(packages.copyInput.SourceDomain).To(Equal("source"))
		Expect(packages.copyInput.SourceProblem).To(Equal("1000"))
		for _, input := range []authoringdomain.CopyInput{
			{SourceDomain: "source", SourceProblem: "1000", Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "1000", SourceVersion: 1},
			{SourceDomain: "../source", SourceProblem: "1000", SourceVersion: 1, Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "not-a-uuid", SourceVersion: 1, Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "1000", SourceVersion: 1, Attribution: strings.Repeat("a", 4097)},
		} {
			packages.copyInput = nil
			_, err := service.Copy(ctx, input)
			Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
			Expect(packages.copyInput).To(BeNil())
		}
	})
})
