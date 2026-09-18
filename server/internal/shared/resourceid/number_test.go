package resourceid

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

func TestCanonicalPublicNumbers(t *testing.T) {
	for _, raw := range []string{"", "0", "0001", "-1", "1e3", "9223372036854775808", "00000000-0000-4000-8000-000000000001"} {
		if _, err := ParseNumber(raw); err == nil {
			t.Errorf("invalid public number accepted: %q", raw)
		}
	}
	for _, raw := range []string{"1", "1000", "9223372036854775807"} {
		n, err := ParseNumber(raw)
		if err != nil || n.String() != raw {
			t.Errorf("valid number lost: %q %v", raw, err)
		}
	}
}
