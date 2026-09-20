package builder

import (
	"context"
	"fmt"

	"github.com/RimuruChan/Vertex/worker/internal/checker"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

func (check *frozenBuild) verifyValidators(ctx context.Context) error {
	check.report.Stage = "validation-tests"
	for index, item := range check.job.Check.Validation {
		check.reporter.Stage("validation-tests", index, len(check.job.Check.Validation))
		outcome := ValidationOutcome{ID: item.ID, Name: item.Definition.Name, Mode: item.Definition.Mode, Status: StatusOK}
		actual, err := check.validationCase(ctx, item)
		outcome.Actual = actual
		if err != nil {
			outcome.Status = StatusFailed
			outcome.Message = err.Error()
		}
		check.report.Validation = append(check.report.Validation, outcome)
		if err != nil {
			return fmt.Errorf("校验器自测 %s 失败：%w", item.Definition.Name, err)
		}
		check.reporter.Stage("validation-tests", index+1, len(check.job.Check.Validation))
	}
	return nil
}

func (check *frozenBuild) validationCase(ctx context.Context, item FrozenValidation) (string, error) {
	if item.Input == nil {
		return "error", fmt.Errorf("缺少自测输入")
	}
	input, err := check.content(ctx, *item.Input)
	if err != nil {
		return "error", err
	}
	rejected := false
	for _, id := range check.job.Check.Metadata.InputValidators {
		program, exists := check.programs[id]
		if !exists {
			return "error", fmt.Errorf("输入校验器不存在")
		}
		result, err := check.execute(ctx, id, input, "", nil, check.job.Limits.ValidatorTimeMs, check.job.Limits.MemoryLimitKB)
		if err != nil {
			return "error", err
		}
		if result.Meta == nil || !checker.ValidatorExitedCleanly(*result.Meta) {
			return "error", fmt.Errorf("输入校验器 %s 崩溃或超出执行限制", program.definition.Name)
		}
		accepted, err := checker.ValidatorAccepted(*result.Meta, program.definition.Protocol)
		if err != nil {
			return "error", err
		}
		rejected = rejected || !accepted
	}
	if item.Definition.Mode == "invalid_input" {
		if rejected {
			return "rejected", nil
		}
		return "accepted", fmt.Errorf("无效输入被全部输入校验器接受")
	}
	if rejected {
		return "invalid_input", fmt.Errorf("输出自测的输入未通过输入校验")
	}
	if item.Answer == nil || item.Output == nil {
		return "error", fmt.Errorf("缺少自测答案或待验证输出")
	}
	answer, err := check.content(ctx, *item.Answer)
	if err != nil {
		return "error", err
	}
	output, err := check.content(ctx, *item.Output)
	if err != nil {
		return "error", err
	}
	decision, err := check.compare(ctx, input, output, answer)
	if err != nil {
		return "error", err
	}
	actual := "error"
	if decision.Verdict == verdict.AC {
		actual = "accepted"
	} else if decision.Verdict == verdict.WA {
		actual = "rejected"
	}
	if actual == "error" {
		return actual, fmt.Errorf("输出校验器未正常判定：%s %s", decision.Message, decision.JudgeMessage)
	}
	if item.Definition.Mode == "valid_output" && actual != "accepted" {
		return actual, fmt.Errorf("应接受的输出被拒绝：%s %s", decision.Message, decision.JudgeMessage)
	}
	if item.Definition.Mode == "invalid_output" && actual != "rejected" {
		return actual, fmt.Errorf("应拒绝的输出被接受")
	}
	return actual, nil
}
