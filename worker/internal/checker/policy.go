package checker

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

// Policy is part of the immutable judging input. FloatingPoint is explicit:
// disabled comparison and an explicitly requested zero tolerance differ.
type Policy struct {
	Kind              string  `json:"kind"`
	CaseSensitive     bool    `json:"caseSensitive"`
	SpaceSensitive    bool    `json:"spaceSensitive"`
	FloatingPoint     bool    `json:"floatingPoint"`
	AbsoluteTolerance float64 `json:"absoluteTolerance"`
	RelativeTolerance float64 `json:"relativeTolerance"`
}

func (policy Policy) Validate() error {
	if policy.Kind != "exact" && policy.Kind != "tokens" {
		return fmt.Errorf("unsupported built-in output comparison %q", policy.Kind)
	}
	for _, value := range []float64{policy.AbsoluteTolerance, policy.RelativeTolerance} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("invalid floating-point tolerance")
		}
	}
	if (policy.Kind != "tokens" || !policy.FloatingPoint) && (policy.AbsoluteTolerance != 0 || policy.RelativeTolerance != 0) {
		return fmt.Errorf("floating-point tolerances require enabled token comparison")
	}
	if policy.Kind == "exact" && policy.FloatingPoint {
		return fmt.Errorf("exact comparison cannot use floating-point tolerance")
	}
	return nil
}

func CheckOutput(actualPath, expectedPath string, policy Policy) (Verdict, error) {
	if err := policy.Validate(); err != nil {
		return Verdict{}, err
	}
	actual, err := readFile(actualPath)
	if err != nil {
		return Verdict{Verdict: verdict.SE, Message: "cannot read program output"}, err
	}
	expected, err := readFile(expectedPath)
	if err != nil {
		return Verdict{Verdict: verdict.SE, Message: "cannot read answer file"}, err
	}
	return CompareOutput(actual, expected, policy)
}

// CompareOutput implements exact bytes or Kattis-style tokens. Token comparison
// uses ASCII whitespace/case rules, matching the reference validator's C locale;
// it does not apply Unicode equivalences to arbitrary program output.
func CompareOutput(actual, expected []byte, policy Policy) (Verdict, error) {
	if err := policy.Validate(); err != nil {
		return Verdict{}, err
	}
	if int64(len(actual)) > MaxOutputBytes || int64(len(expected)) > MaxOutputBytes {
		return Verdict{}, fmt.Errorf("output comparison exceeds size limit")
	}
	if policy.Kind == "exact" {
		if bytes.Equal(actual, expected) {
			return Verdict{Verdict: verdict.AC, Message: "ok"}, nil
		}
		return Verdict{Verdict: verdict.WA, Message: "output differs from the answer"}, nil
	}
	actualPos, expectedPos := 0, 0
	for index := 1; ; index++ {
		aSpace, aToken, aNext := nextOutputToken(actual, actualPos)
		eSpace, eToken, eNext := nextOutputToken(expected, expectedPos)
		if policy.SpaceSensitive && !bytes.Equal(aSpace, eSpace) {
			return Verdict{Verdict: verdict.WA, Message: fmt.Sprintf("whitespace differs before token %d", index)}, nil
		}
		if len(aToken) == 0 || len(eToken) == 0 {
			if len(aToken) == len(eToken) {
				return Verdict{Verdict: verdict.AC, Message: "ok"}, nil
			}
			return Verdict{Verdict: verdict.WA, Message: fmt.Sprintf("output token count differs at token %d", index)}, nil
		}
		if !outputTokensEqual(aToken, eToken, policy) {
			return Verdict{Verdict: verdict.WA, Message: fmt.Sprintf("output differs at token %d", index)}, nil
		}
		actualPos, expectedPos = aNext, eNext
	}
}

func outputTokensEqual(actual, expected []byte, policy Policy) bool {
	if bytes.Equal(actual, expected) {
		return true
	}
	if policy.FloatingPoint {
		if expectedNumber, ok := finiteFloat(expected); ok {
			actualNumber, ok := finiteFloat(actual)
			if !ok {
				return false
			}
			difference := math.Abs(expectedNumber - actualNumber)
			return difference <= policy.AbsoluteTolerance || difference <= policy.RelativeTolerance*math.Abs(expectedNumber)
		}
	}
	return !policy.CaseSensitive && asciiEqualFold(actual, expected)
}

func nextOutputToken(data []byte, start int) (space, token []byte, next int) {
	position := start
	for position < len(data) && outputSpace(data[position]) {
		position++
	}
	space = data[start:position]
	start = position
	for position < len(data) && !outputSpace(data[position]) {
		position++
	}
	return space, data[start:position], position
}
func outputSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\n' || value == '\r' || value == '\v' || value == '\f'
}
func asciiEqualFold(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i, x := range a {
		y := b[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

var decimalOutput = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
var hexOutput = regexp.MustCompile(`^[+-]?0[xX](?:[0-9a-fA-F]+(?:\.[0-9a-fA-F]*)?|\.[0-9a-fA-F]+)(?:[pP][+-]?[0-9]+)?$`)

func finiteFloat(token []byte) (float64, bool) {
	text := string(token)
	if !decimalOutput.Match(token) {
		if !hexOutput.Match(token) {
			return 0, false
		}
		if !strings.ContainsAny(text, "pP") {
			text += "p0"
		}
	}
	value, err := strconv.ParseFloat(text, 64)
	return value, err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}
