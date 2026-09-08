package domain

import "fmt"

const MaxResultCases = 10000

func ValidateResult(result Result) error {
	if !validVerdict(result.Status) {
		return fmt.Errorf("%w: unsupported status %q", ErrInvalidResult, result.Status)
	}
	if result.Score < 0 || result.Score > 100 || result.TotalTimeMs < 0 || result.PeakMemoryKB < 0 {
		return fmt.Errorf("%w: negative resource usage or score outside 0-100", ErrInvalidResult)
	}
	if len(result.Cases) > MaxResultCases {
		return fmt.Errorf("%w: case result count exceeds %d", ErrInvalidResult, MaxResultCases)
	}
	caseIndexes := make(map[int]struct{}, len(result.Cases))
	for _, item := range result.Cases {
		if item.CaseIndex <= 0 || item.TimeMs < 0 || item.MemoryKB < 0 || !validVerdict(item.Verdict) {
			return fmt.Errorf("%w: invalid case result", ErrInvalidResult)
		}
		if _, duplicate := caseIndexes[item.CaseIndex]; duplicate {
			return fmt.Errorf("%w: duplicate case index %d", ErrInvalidResult, item.CaseIndex)
		}
		caseIndexes[item.CaseIndex] = struct{}{}
	}
	return nil
}

func validVerdict(status string) bool {
	switch status {
	case "Accepted", "Wrong Answer", "Time Limit Exceeded", "Memory Limit Exceeded",
		"Runtime Error", "Compile Error", "Output Limit Exceeded", "System Error", "Skipped":
		return true
	default:
		return false
	}
}
