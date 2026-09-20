package checker

import (
	"math"
	"testing"

	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

func TestOutputPolicies(t *testing.T) {
	for _, test := range []struct {
		name, actual, answer string
		policy               Policy
		accepted             bool
	}{
		{"kattis-default", "YES\n 42\t", "yes 42\n", Policy{Kind: "tokens"}, true},
		{"case-sensitive", "YES", "yes", Policy{Kind: "tokens", CaseSensitive: true}, false},
		{"space-sensitive", "a  b\n", "a b\n", Policy{Kind: "tokens", SpaceSensitive: true}, false},
		{"trailing-space-sensitive", "a\n\n", "a\n", Policy{Kind: "tokens", SpaceSensitive: true}, false},
		{"tokens-default", " a \t b \n", "a b", Policy{Kind: "tokens"}, true},
		{"exact-newline", "42", "42\n", Policy{Kind: "exact"}, false},
		{"exact-binary", "x\x00z", "x\x00z", Policy{Kind: "exact"}, true},
		{"embedded-nul-not-truncated", "x\x00extra", "x", Policy{Kind: "tokens"}, false},
		{"no-numeric-mode", "1.0", "1", Policy{Kind: "tokens"}, false},
		{"explicit-zero", "1.0", "1", Policy{Kind: "tokens", FloatingPoint: true}, true},
		{"relative", "100.005", "100", Policy{Kind: "tokens", FloatingPoint: true, RelativeTolerance: 0.0001}, true},
		{"absolute-or-relative", "0.00001", "0", Policy{Kind: "tokens", FloatingPoint: true, AbsoluteTolerance: 0.0001, RelativeTolerance: 0.00001}, true},
		{"relative-is-not-scaled-to-one", "0.00001", "0", Policy{Kind: "tokens", FloatingPoint: true, RelativeTolerance: 0.0001}, false},
		{"scientific", "3.14e-2", "0.0314", Policy{Kind: "tokens", FloatingPoint: true}, true},
		{"hexadecimal", "0x10", "16", Policy{Kind: "tokens", FloatingPoint: true}, true},
		{"go-underscores-rejected", "1_000", "1000", Policy{Kind: "tokens", FloatingPoint: true}, false},
		{"nonfinite-not-number", "NaN", "0", Policy{Kind: "tokens", FloatingPoint: true}, false},
		{"nonfinite-as-literal", "NaN", "nan", Policy{Kind: "tokens", FloatingPoint: true}, true},
		{"overflow", "1e10000", "1", Policy{Kind: "tokens", FloatingPoint: true}, false},
		{"unicode-case-not-ascii", "K", "k", Policy{Kind: "tokens"}, false},
		{"nonbreaking-space-not-token-boundary", "a\u00a0b", "a b", Policy{Kind: "tokens"}, false},
		{"extra-token", "1 2", "1", Policy{Kind: "tokens"}, false},
		{"empty", " \n", "", Policy{Kind: "tokens"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := CompareOutput([]byte(test.actual), []byte(test.answer), test.policy)
			if err != nil {
				t.Fatal(err)
			}
			if (got.Verdict == verdict.AC) != test.accepted {
				t.Fatalf("comparison = %+v, wanted accepted=%v", got, test.accepted)
			}
		})
	}
	for _, policy := range []Policy{{Kind: "unknown"}, {Kind: "tokens", AbsoluteTolerance: 1}, {Kind: "tokens", FloatingPoint: true, RelativeTolerance: math.Inf(1)}, {Kind: "exact", FloatingPoint: true}} {
		if _, err := CompareOutput(nil, nil, policy); err == nil {
			t.Fatalf("invalid policy accepted: %+v", policy)
		}
	}
}
