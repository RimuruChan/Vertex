package domain

import "strings"

type StatementPreviewInput struct {
	ETag     string `json:"etag"`
	Revision int64  `json:"revision"`
	EntryID  string `json:"entryId"`
	Content  string `json:"content"`
}
type DraftStatementPreview struct {
	Content     string   `json:"content"`
	Warnings    []string `json:"warnings"`
	SampleCount int      `json:"sampleCount"`
}

func StandardStatement(title, language string) string {
	title = strings.ReplaceAll(strings.ReplaceAll(title, "\r", " "), "\n", " ")
	if strings.HasPrefix(language, "zh") {
		return "# " + title + "\n\n## 题目描述\n\n\n## 输入格式\n\n\n## 输出格式\n\n\n{{remainingsamples}}\n\n## 样例说明\n\n\n## 数据范围与提示\n\n"
	}
	return "# " + title + "\n\n## Description\n\n\n## Input\n\n\n## Output\n\n\n{{remainingsamples}}\n\n## Explanation\n\n\n## Constraints\n\n"
}
