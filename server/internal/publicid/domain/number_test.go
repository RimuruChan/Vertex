package domain

import "testing"

func TestDecimalReferences(t *testing.T) {
	for _, value := range []string{"1000", "0001", "0"} {
		if !IsNumber(value) {
			t.Errorf("decimal reference rejected: %q", value)
		}
	}
	for _, value := range []string{"", "A", "-1", "1.2", "00000000-0000-4000-8000-000000000001"} {
		if IsNumber(value) {
			t.Errorf("non-decimal reference accepted: %q", value)
		}
	}
}
