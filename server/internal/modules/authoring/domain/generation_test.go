package domain

import (
	"reflect"
	"testing"
)

func TestGenerationExpansion(t *testing.T) {
	plan := GenerationPlan{SchemaVersion: 1, Name: "规模覆盖", Generator: "gen", Solution: "sol", Rules: []GenerationRule{{ID: "small", Name: "小范围", Count: 3, SeedStart: 7, Parameters: `--n 100 --seed {seed} "case {index}"`}}}
	items, err := ExpandGeneration("plan", plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || !reflect.DeepEqual(items[1].Arguments, []string{"--n", "100", "--seed", "8", "case 2"}) {
		t.Fatalf("bad expansion: %+v", items)
	}
	next, err := ExpandGeneration("plan", plan)
	if err != nil || next[1].ID != items[1].ID {
		t.Fatal("generation IDs must be stable")
	}
	plan.Rules[0].Count = 501
	if _, err := ExpandGeneration("plan", plan); err == nil {
		t.Fatal("unbounded expansion accepted")
	}
}
func TestGenerationArgumentsAreLiteral(t *testing.T) {
	got, err := GenerationArguments(`'a b' "" $(touch /tmp/x) > file`)
	if err != nil || !reflect.DeepEqual(got, []string{"a b", "", "$(touch", "/tmp/x)", ">", "file"}) {
		t.Fatalf("argv must be literal: %q %v", got, err)
	}
	if _, err := GenerationArguments(`"unfinished`); err == nil {
		t.Fatal("unterminated quotes accepted")
	}
}
