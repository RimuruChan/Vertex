package domain

import "errors"

var ErrNotFound = errors.New("public resource not found")

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
