package resourceid

import (
	"errors"
	"strconv"
)

var ErrNotFound = errors.New("public resource not found")

// Number is a canonical, positive, domain-local reference, never an internal UUID.
type Number int64

func ParseNumber(value string) (Number, error) {
	if len(value) == 0 || len(value) > 19 || !IsNumber(value) || value[0] == '0' {
		return 0, ErrNotFound
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0, ErrNotFound
	}
	return Number(n), nil
}
func (n Number) String() string { return strconv.FormatInt(int64(n), 10) }

func OptionalNumber(number string) *string {
	if number == "" {
		return nil
	}
	return &number
}

func IsNumber(ref string) bool {
	if ref == "" {
		return false
	}
	for _, digit := range ref {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}
