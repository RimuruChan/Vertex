package store

import (
	"encoding/json"
)

// jsonMarshal 包装 encoding/json.Marshal,错误统一冒泡。
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal 包装 encoding/json.Unmarshal。
func jsonUnmarshal(data []byte, v any) error {
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, v)
}
