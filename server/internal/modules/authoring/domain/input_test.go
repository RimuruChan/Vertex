package domain

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseGenerateCommand", func() {
	It("splits the generator name from its arguments", func() {
		name, arguments, err := ParseGenerateCommand("  gen_random  1000   -seed=42 ")
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("gen_random"))
		Expect(arguments).To(Equal([]string{"1000", "-seed=42"}))
	})

	It("rejects shell metacharacters so a command can never reach a shell", func() {
		for _, command := range []string{"gen 1; rm -rf /", "gen $(id)", "gen `id`", "gen 'a b'"} {
			_, _, err := ParseGenerateCommand(command)
			Expect(err).To(MatchError(ErrInvalidInput), command)
		}
	})

	It("rejects an empty command", func() {
		_, _, err := ParseGenerateCommand("   ")
		Expect(err).To(MatchError(ErrInvalidInput))
	})
})

var _ = Describe("Validate", func() {
	It("requires tests and a main solution", func() {
		issues := Validate(&Package{})
		Expect(issues).To(HaveLen(2))
	})

	It("reports generator commands that reference a missing generator", func() {
		issues := Validate(&Package{
			Solutions: []File{{Name: "std", Kind: KindSolution, IsActive: true}},
			Tests:     []Test{{Index: 1, Source: TestGenerator, GenerateCmd: "missing 10"}},
		})
		Expect(issues).To(HaveLen(1))
		Expect(issues[0]).To(ContainSubstring("missing"))
	})

	It("accepts a package whose generators all resolve", func() {
		issues := Validate(&Package{
			Solutions:  []File{{Name: "std", Kind: KindSolution, IsActive: true}},
			Generators: []File{{Name: "gen", Kind: KindGenerator}},
			Tests: []Test{
				{Index: 1, Source: TestManual, InputData: "1"},
				{Index: 2, Source: TestGenerator, GenerateCmd: "gen 10"},
			},
		})
		Expect(issues).To(BeEmpty())
	})

	It("requires an interactor for interactive problems", func() {
		issues := Validate(&Package{
			JudgeType: "interactive",
			Solutions: []File{{Name: "std", Kind: KindSolution, IsActive: true}},
			Tests:     []Test{{Index: 1, Source: TestManual, InputData: "1"}},
		})
		Expect(issues).To(ContainElement(ContainSubstring("interactor")))
	})
})
