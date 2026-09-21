package domain

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const EntryGeneration = "generation"

// GenerationPlan records author intent; applying it replaces only its own
// expanded tests. Worker execution consumes the frozen per-test arguments.
type GenerationPlan struct {
	SchemaVersion int              `json:"schemaVersion"`
	Name          string           `json:"name"`
	Generator     string           `json:"generator"`
	Solution      string           `json:"solution"`
	Group         string           `json:"group"`
	Rules         []GenerationRule `json:"rules"`
}
type GenerationRule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
	SeedStart  int64  `json:"seedStart"`
	Parameters string `json:"parameters"`
}
type GenerationInput struct {
	ETag    string         `json:"etag"`
	ID      string         `json:"id"`
	Plan    GenerationPlan `json:"plan"`
	Preview bool           `json:"preview"`
}
type GeneratedCase struct {
	ID        string   `json:"id"`
	RuleID    string   `json:"ruleId"`
	Name      string   `json:"name"`
	Arguments []string `json:"arguments"`
	Seed      int64    `json:"seed"`
	Position  int      `json:"position"`
}
type GenerationResult struct {
	ID    string          `json:"id"`
	Cases []GeneratedCase `json:"cases"`
	Copy  *WorkingCopy    `json:"copy,omitempty"`
}

func (plan *GenerationPlan) Validate() error {
	if plan.SchemaVersion != MaterialSchemaVersion || strings.TrimSpace(plan.Name) == "" || len(plan.Name) > 256 || !contentIDPattern.MatchString(plan.Generator) || !contentIDPattern.MatchString(plan.Solution) || (plan.Group != "" && !contentIDPattern.MatchString(plan.Group)) || len(plan.Rules) == 0 || len(plan.Rules) > 50 {
		return InvalidInput("生成方案的名称、程序或规则无效")
	}
	seen := map[string]bool{}
	total := 0
	for _, rule := range plan.Rules {
		if !contentIDPattern.MatchString(rule.ID) || seen[rule.ID] || strings.TrimSpace(rule.Name) == "" || len(rule.Name) > 128 || rule.Count < 1 || rule.Count > 500 || rule.SeedStart < 0 || rule.SeedStart > 9007199254740000 || len(rule.Parameters) > 8192 {
			return InvalidInput("生成规则无效：每条需要名称、1–500 个测试和有效起始种子")
		}
		seen[rule.ID] = true
		total += rule.Count
		if _, err := GenerationArguments(rule.Parameters); err != nil {
			return err
		}
	}
	if total > 500 {
		return InvalidInput("一个生成方案最多展开 500 个测试点")
	}
	return nil
}

// This is an argv lexer, not a shell. Quotes group arguments; nothing executes,
// expands environment variables, redirects output, or invokes another process.
func GenerationArguments(text string) ([]string, error) {
	result := []string{}
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	flush := func() {
		if started {
			result = append(result, word.String())
			word.Reset()
			started = false
		}
	}
	for _, r := range text {
		if r == 0 {
			return nil, InvalidInput("参数不能包含空字符")
		}
		if escaped {
			word.WriteRune(r)
			escaped = false
			started = true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if unicode.IsSpace(r) {
			flush()
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if escaped || quote != 0 {
		return nil, InvalidInput("参数的引号或转义没有结束")
	}
	flush()
	if len(result) > 64 {
		return nil, InvalidInput("每次调用最多 64 个参数")
	}
	for _, arg := range result {
		if len(arg) > 1024 {
			return nil, InvalidInput("单个参数最多 1024 字节")
		}
	}
	return result, nil
}
func ExpandGeneration(id string, plan GenerationPlan) ([]GeneratedCase, error) {
	if !contentIDPattern.MatchString(id) {
		return nil, InvalidInput("生成方案标识无效")
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	result := []GeneratedCase{}
	for _, rule := range plan.Rules {
		args, _ := GenerationArguments(rule.Parameters)
		for i := 0; i < rule.Count; i++ {
			seed := rule.SeedStart + int64(i)
			expanded := make([]string, len(args))
			for j, arg := range args {
				expanded[j] = strings.NewReplacer("{seed}", strconv.FormatInt(seed, 10), "{index}", strconv.Itoa(i+1)).Replace(arg)
			}
			result = append(result, GeneratedCase{ID: "generated-" + Digest([]byte(fmt.Sprintf("%s\x00%s\x00%d", id, rule.ID, i)))[:32], RuleID: rule.ID, Name: fmt.Sprintf("%s · %d", rule.Name, i+1), Arguments: expanded, Seed: seed, Position: len(result) + 1})
		}
	}
	return result, nil
}
