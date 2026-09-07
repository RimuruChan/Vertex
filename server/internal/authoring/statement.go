package authoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// Sample is one statement example. Samples are never authored by hand: they
// are the input/answer pair a successful build produced for a test marked as a
// sample, so the statement can never show an example the judge would reject.
type Sample struct {
	Index  int
	Input  string
	Answer string
}

// StatementSections are the localized headings used when rendering a package
// statement into the public Markdown blob.
type StatementSections struct {
	Legend       string
	InputFormat  string
	OutputFormat string
	Samples      string
	Sample       string
	Input        string
	Output       string
	Notes        string
	Scoring      string
}

var statementHeadings = map[string]StatementSections{
	"zh": {
		Legend: "题目描述", InputFormat: "输入格式", OutputFormat: "输出格式",
		Samples: "样例", Sample: "样例", Input: "输入", Output: "输出",
		Notes: "说明与提示", Scoring: "计分方式",
	},
	"en": {
		Legend: "Statement", InputFormat: "Input", OutputFormat: "Output",
		Samples: "Examples", Sample: "Example", Input: "Input", Output: "Output",
		Notes: "Notes", Scoring: "Scoring",
	},
}

func headingsFor(language string) StatementSections {
	if sections, ok := statementHeadings[strings.ToLower(language)]; ok {
		return sections
	}
	return statementHeadings["en"]
}

// RenderStatement turns a structured statement plus built samples into the
// Markdown that the public problem page renders. Sample blocks use fenced code
// so leading whitespace in test data survives Markdown rendering intact.
func RenderStatement(statement Statement, samples []Sample) string {
	sections := headingsFor(statement.Language)
	var out strings.Builder

	writeSection := func(heading, body string) {
		body = strings.TrimRight(body, " \t\r\n")
		if body == "" {
			return
		}
		out.WriteString("## ")
		out.WriteString(heading)
		out.WriteString("\n\n")
		out.WriteString(body)
		out.WriteString("\n\n")
	}

	writeSection(sections.Legend, statement.Legend)
	writeSection(sections.InputFormat, statement.InputFormat)
	writeSection(sections.OutputFormat, statement.OutputFormat)

	if len(samples) > 0 {
		out.WriteString("## ")
		out.WriteString(sections.Samples)
		out.WriteString("\n\n")
		for position, sample := range samples {
			if len(samples) > 1 {
				out.WriteString("### ")
				out.WriteString(sections.Sample)
				out.WriteString(" ")
				out.WriteString(itoa(position + 1))
				out.WriteString("\n\n")
			}
			out.WriteString("**")
			out.WriteString(sections.Input)
			out.WriteString("**\n\n")
			writeFence(&out, sample.Input)
			out.WriteString("**")
			out.WriteString(sections.Output)
			out.WriteString("**\n\n")
			writeFence(&out, sample.Answer)
		}
	}

	writeSection(sections.Scoring, statement.Scoring)
	writeSection(sections.Notes, statement.Notes)
	return strings.TrimRight(out.String(), "\n") + "\n"
}

// writeFence emits a fenced block whose delimiter is always longer than any
// backtick run inside the sample, so test data containing backticks cannot
// break out of the code block.
func writeFence(out *strings.Builder, body string) {
	fence := strings.Repeat("`", longestBacktickRun(body)+1)
	if len(fence) < 3 {
		fence = "```"
	}
	out.WriteString(fence)
	out.WriteString("\n")
	out.WriteString(strings.TrimRight(body, "\n"))
	out.WriteString("\n")
	out.WriteString(fence)
	out.WriteString("\n\n")
}

func longestBacktickRun(value string) int {
	longest, current := 0, 0
	for _, char := range value {
		if char == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 8)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// SamplesFromOutcomes extracts the statement examples from a build report.
// The worker sends complete text for sample tests and only a bounded head for
// the rest, so filtering on IsSample is what keeps this honest.
func SamplesFromOutcomes(tests []TestOutcome) []Sample {
	samples := make([]Sample, 0, 4)
	for _, item := range tests {
		if !item.IsSample {
			continue
		}
		samples = append(samples, Sample{
			Index: item.Index, Input: item.InputHead, Answer: item.AnswerHead,
		})
	}
	return samples
}

// Rendering a working statement never changes the published projection.
func renderWorkspaceStatementTx(ctx context.Context, tx execQueryer, problemID string, tests []TestOutcome) error {
	var language string
	if err := tx.QueryRowContext(ctx,
		`SELECT statement_language FROM problem_workspaces WHERE problem_id = $1`, problemID).Scan(&language); err != nil {
		return err
	}
	statement, err := statementFrom(ctx, tx, problemID, language)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	rendered := RenderStatement(*statement, SamplesFromOutcomes(tests))
	_, err = tx.ExecContext(ctx,
		`UPDATE problem_workspaces SET statement_md = $2, title = COALESCE(NULLIF($3::text, ''), title), updated_at = now()
		 WHERE problem_id = $1`, problemID, rendered, statement.Name)
	return err
}

func statementFrom(ctx context.Context, q queryer, problemID, language string) (*Statement, error) {
	var item Statement
	err := q.QueryRowContext(ctx,
		`SELECT problem_id, language, name, legend, input_format, output_format,
		        notes, tutorial, scoring, updated_at
		 FROM problem_statements WHERE problem_id = $1 AND language = $2`,
		problemID, language).Scan(&item.ProblemID, &item.Language, &item.Name, &item.Legend,
		&item.InputFormat, &item.OutputFormat, &item.Notes, &item.Tutorial,
		&item.Scoring, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// Candidate samples are a preview, not a publication side effect.
func lastBuiltSamples(ctx context.Context, q queryer, problemID string) ([]TestOutcome, error) {
	var payload []byte
	err := q.QueryRowContext(ctx,
		`SELECT samples_json FROM problem_testdata WHERE problem_id=$1`, problemID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var tests []TestOutcome
	if err := json.Unmarshal(payload, &tests); err != nil {
		return nil, nil
	}
	return tests, nil
}
