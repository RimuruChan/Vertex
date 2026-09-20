package domain

const EntryValidation = "validation"
const MaxValidationCases = 1000

// ValidationMaterial describes validator self-tests, never judge test cases.
// Valid input is checked for every output self-test and ordinary judge test.
type ValidationMaterial struct {
	SchemaVersion int    `json:"schemaVersion"`
	Name          string `json:"name"`
	Mode          string `json:"mode" enums:"invalid_input,invalid_output,valid_output"`
	Input         string `json:"input"`
	Answer        string `json:"answer,omitempty"`
	Output        string `json:"output,omitempty"`
	Description   string `json:"description,omitempty"`
}

func (value *ValidationMaterial) Validate() error {
	if err := materialIdentity(value.SchemaVersion, value.Name); err != nil {
		return err
	}
	switch value.Mode {
	case "invalid_input":
		if value.Answer != "" || value.Output != "" {
			return InvalidInput("无效输入自测不应包含答案或待验证输出")
		}
	case "invalid_output", "valid_output":
	default:
		return InvalidInput("未知校验器自测类型")
	}
	for _, id := range []string{value.Input, value.Answer, value.Output} {
		if id != "" && !contentIDPattern.MatchString(id) {
			return InvalidInput("无效的自测文件引用")
		}
	}
	if len(value.Description) > 4096 {
		return InvalidInput("自测说明超过长度限制")
	}
	return nil
}

type SnapshotValidation struct {
	ID         string             `json:"id"`
	Definition ValidationMaterial `json:"definition"`
	Input      *BlobRef           `json:"input,omitempty"`
	Answer     *BlobRef           `json:"answer,omitempty"`
	Output     *BlobRef           `json:"output,omitempty"`
}

type ValidationOutcome struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Mode    string `json:"mode"`
	Actual  string `json:"actual"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (check *materialInspector) prepareValidation(tree ContentTree) {
	for _, entry := range tree.Entries {
		definition, exists := check.validation[entry.ID]
		if !exists {
			continue
		}
		prepared := SnapshotValidation{ID: entry.ID, Definition: definition}
		if file := check.requireEntry(definition.Input, EntryInput, entry.ID, "input"); file != nil {
			ref := file.Blob
			prepared.Input = &ref
		}
		if definition.Mode == "invalid_input" {
			if len(check.snapshot.Metadata.InputValidators) == 0 {
				check.issue("error", "validation.validator_required", entry.ID, "input", "无效输入自测至少需要一个输入校验器")
			}
		} else {
			if file := check.requireEntry(definition.Answer, EntryAnswer, entry.ID, "answer"); file != nil {
				ref := file.Blob
				prepared.Answer = &ref
			}
			if file := check.requireEntry(definition.Output, EntryAnswer, entry.ID, "output"); file != nil {
				ref := file.Blob
				prepared.Output = &ref
			}
		}
		check.snapshot.Validation = append(check.snapshot.Validation, prepared)
	}
	if len(check.snapshot.Validation) > MaxValidationCases {
		check.issue("error", "validation.case_limit", "problem", "", "校验器自测最多 1000 项")
	}
	if (len(check.snapshot.Tests)+len(check.snapshot.Validation))*len(check.snapshot.Metadata.InputValidators) > 100000 {
		check.issue("error", "validation.matrix_limit", "problem", "", "输入校验执行矩阵超过 100000 次")
	}
	check.report.ValidationCount = len(check.snapshot.Validation)
}
