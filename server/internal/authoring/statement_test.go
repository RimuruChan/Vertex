package authoring

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("RenderStatement", func() {
	It("renders sections in reading order and skips empty ones", func() {
		rendered := RenderStatement(Statement{
			Language: "zh", Name: "A + B",
			Legend:       "给定两个整数。",
			InputFormat:  "一行两个整数。",
			OutputFormat: "输出它们的和。",
		}, nil)

		Expect(rendered).To(ContainSubstring("## 题目描述"))
		Expect(rendered).To(ContainSubstring("## 输入格式"))
		Expect(rendered).To(ContainSubstring("## 输出格式"))
		Expect(rendered).NotTo(ContainSubstring("## 说明与提示"))
		Expect(strings.Index(rendered, "## 题目描述")).To(BeNumerically("<", strings.Index(rendered, "## 输入格式")))
	})

	It("falls back to English headings for unknown languages", func() {
		rendered := RenderStatement(Statement{Language: "fr", Legend: "Bonjour"}, nil)
		Expect(rendered).To(ContainSubstring("## Statement"))
	})

	It("numbers multiple samples and fences their exact bytes", func() {
		rendered := RenderStatement(Statement{Language: "zh", Legend: "x"}, []Sample{
			{Index: 1, Input: "1 2", Answer: "3"},
			{Index: 2, Input: "  4   5  ", Answer: "9"},
		})
		Expect(rendered).To(ContainSubstring("### 样例 1"))
		Expect(rendered).To(ContainSubstring("### 样例 2"))
		Expect(rendered).To(ContainSubstring("```\n  4   5  \n```"))
	})

	It("keeps a single sample unnumbered", func() {
		rendered := RenderStatement(Statement{Language: "zh"}, []Sample{{Index: 1, Input: "1", Answer: "1"}})
		Expect(rendered).To(ContainSubstring("## 样例"))
		Expect(rendered).NotTo(ContainSubstring("### 样例 1"))
	})

	It("widens the fence so sample data cannot escape the code block", func() {
		rendered := RenderStatement(Statement{Language: "zh"}, []Sample{
			{Index: 1, Input: "``` not a fence", Answer: "ok"},
		})
		Expect(rendered).To(ContainSubstring("````\n``` not a fence\n````"))
	})
})

var _ = Describe("SamplesFromOutcomes", func() {
	It("keeps only tests the build marked as samples, in report order", func() {
		samples := SamplesFromOutcomes([]TestOutcome{
			{Index: 1, IsSample: true, InputHead: "1", AnswerHead: "1"},
			{Index: 2, IsSample: false, InputHead: "big", AnswerHead: "big"},
			{Index: 3, IsSample: true, InputHead: "3", AnswerHead: "9"},
		})
		Expect(samples).To(HaveLen(2))
		Expect(samples[0].Index).To(Equal(1))
		Expect(samples[1].Answer).To(Equal("9"))
	})
})
