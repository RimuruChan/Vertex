package domain

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RenderSamples", func() {
	It("uses English for unknown languages and omits an empty section", func() {
		Expect(RenderSamples("fr", []Sample{{Input: "1", Answer: "2"}})).To(ContainSubstring("## Examples"))
		Expect(RenderSamples("zh", nil)).To(BeEmpty())
	})
	It("numbers multiple samples and fences their exact bytes", func() {
		rendered := RenderSamples("zh", []Sample{
			{Index: 1, Input: "1 2", Answer: "3"},
			{Index: 2, Input: "  4   5  ", Answer: "9"},
		})
		Expect(rendered).To(ContainSubstring("### 样例 1"))
		Expect(rendered).To(ContainSubstring("### 样例 2"))
		Expect(rendered).To(ContainSubstring("```\n  4   5  \n```"))
	})

	It("keeps a single sample unnumbered", func() {
		rendered := RenderSamples("zh", []Sample{{Index: 1, Input: "1", Answer: "1"}})
		Expect(rendered).To(ContainSubstring("## 样例"))
		Expect(rendered).NotTo(ContainSubstring("### 样例 1"))
	})

	It("widens the fence so sample data cannot escape the code block", func() {
		rendered := RenderSamples("zh", []Sample{
			{Index: 1, Input: "``` not a fence", Answer: "ok"},
		})
		Expect(rendered).To(ContainSubstring("````\n``` not a fence\n````"))
	})
})

func TestAuthoringDomain(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "AuthoringDomain") }
