package domain

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"strings"
)

var _ = Describe("Copy input rules", func() {
	It("normalizes references and rejects missing release or attribution before persistence", func() {
		normalized, err := NormalizeCopy(CopyInput{SourceDomain: " source ", SourceProblem: "001000", SourceVersion: 1, Attribution: " Approved source "})
		Expect(err).NotTo(HaveOccurred())
		Expect(normalized.SourceDomain).To(Equal("source"))
		Expect(normalized.SourceProblem).To(Equal("1000"))
		for _, input := range []CopyInput{
			{SourceDomain: "source", SourceProblem: "1000", Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "1000", SourceVersion: 1},
			{SourceDomain: "../source", SourceProblem: "1000", SourceVersion: 1, Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "not-a-uuid", SourceVersion: 1, Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "1000", SourceVersion: 1, Attribution: strings.Repeat("a", 4097)},
		} {
			_, err := NormalizeCopy(input)
			Expect(err).To(MatchError(ErrInvalidInput))
		}
	})
})
