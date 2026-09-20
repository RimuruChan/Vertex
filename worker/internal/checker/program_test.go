package checker

import (
	"testing"

	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

func TestValidatorProtocolExitCodes(t *testing.T) {
	for _, test := range []struct {
		protocol string
		meta     run.Meta
		want     string
	}{
		{"kattis", run.Meta{ExitCode: 42, Status: "RE", TerminationReason: run.TerminationExited}, verdict.AC},
		{"kattis", run.Meta{ExitCode: 43, Status: "RE"}, verdict.WA},
		{"kattis", run.Meta{ExitCode: 0}, verdict.SE},
		{"kattis", run.Meta{ExitCode: 1}, verdict.SE},
		{"kattis", run.Meta{ExitCode: 42, TerminationReason: run.TerminationTimeLimit}, verdict.SE},
		{"kattis", run.Meta{ExitCode: 42, ExitSignal: 11}, verdict.SE},
		{"testlib", run.Meta{ExitCode: 0}, verdict.AC},
		{"testlib", run.Meta{ExitCode: 1}, verdict.WA},
		{"testlib", run.Meta{ExitCode: 3}, verdict.SE},
		{"testlib", run.Meta{ExitCode: 7}, verdict.SE},
		{"testlib", run.Meta{ExitCode: 0, Killed: true}, verdict.SE},
	} {
		if got := validatorDecision(test.meta, test.protocol); got.Verdict != test.want {
			t.Errorf("%s %+v => %+v; want %s", test.protocol, test.meta, got, test.want)
		}
	}
	for _, test := range []struct {
		protocol string
		meta     run.Meta
		ok       bool
	}{
		{"kattis", run.Meta{ExitCode: 42}, true}, {"kattis", run.Meta{ExitCode: 0}, false}, {"testlib", run.Meta{ExitCode: 0}, true}, {"stdio", run.Meta{ExitCode: 1}, false}, {"kattis", run.Meta{ExitCode: 42, CgOOMKilled: true}, false},
	} {
		got, err := ValidatorAccepted(test.meta, test.protocol)
		if err != nil || got != test.ok {
			t.Fatalf("input validator %s %+v: %v %v", test.protocol, test.meta, got, err)
		}
	}
	if _, err := ValidatorAccepted(run.Meta{}, "unknown"); err == nil {
		t.Fatal("unknown validator protocol accepted")
	}
}
