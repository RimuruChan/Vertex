package domain

import (
	"encoding/json"
	"testing"
)

func TestInitialMaterialsAndDocumentRoundTrip(t *testing.T) {
	materials, err := InitialMaterials("A + B", "Find the sum.\n", "en", "Practice", "normal", 1000, 262144)
	if err != nil {
		t.Fatal(err)
	}
	if len(materials) != 2 {
		t.Fatalf("seeded %d materials", len(materials))
	}
	for _, material := range materials {
		if err := ValidateBlob(material.Entry.Blob, material.Data); err != nil {
			t.Fatal(err)
		}
	}
	view, err := DecodeMaterial(materials[0].Entry, materials[0].Data)
	if err != nil || view.Metadata == nil || view.Metadata.Title != "A + B" || view.Metadata.Comparison.Kind != "tokens" {
		t.Fatalf("bad initial metadata: %+v %v", view, err)
	}
	for _, item := range []struct {
		kind  string
		value any
	}{
		{EntryProgram, ProgramMaterial{SchemaVersion: 1, Name: "Reference", Role: "solution", Language: "cpp", Protocol: "stdio", Files: []string{"source1"}, EntryPoint: "source1", ExpectedVerdicts: []string{"Accepted"}}},
		{EntryTest, TestMaterial{SchemaVersion: 1, Name: "Sample", IsSample: true, Input: TestInput{Kind: "file", Entry: "input1"}, Answer: TestAnswer{Kind: "file", Entry: "answer1"}}},
		{EntryGroup, GroupMaterial{SchemaVersion: 1, Name: "Small values", Aggregation: "pass-fail", MaxScore: 20, Prerequisites: []string{"samples"}}},
	} {
		data, _ := json.Marshal(item.value)
		first, err := NormalizeMaterial(item.kind, data)
		if err != nil {
			t.Fatal(err)
		}
		second, err := NormalizeMaterial(item.kind, first)
		if err != nil || string(first) != string(second) {
			t.Fatal("material encoding is not stable")
		}
	}
}

func TestMaterialValidationPreservesSemantics(t *testing.T) {
	for _, test := range []struct{ name, kind, data string }{
		{"future-schema", EntryGroup, `{"schemaVersion":2,"name":"g","aggregation":"sum"}`},
		{"unknown-field", EntryGroup, `{"schemaVersion":1,"name":"g","aggregation":"sum","silentlyLostRule":true}`},
		{"score", EntryGroup, `{"schemaVersion":1,"name":"g","aggregation":"sum","maxScore":-1}`},
		{"duplicate-dependency", EntryGroup, `{"schemaVersion":1,"name":"g","aggregation":"sum","prerequisites":["a","a"]}`},
		{"wrong-entry-point", EntryProgram, `{"schemaVersion":1,"name":"p","role":"solution","language":"cpp","protocol":"stdio","files":["a"],"entryPoint":"b"}`},
		{"wrong-protocol", EntryProgram, `{"schemaVersion":1,"name":"p","role":"solution","language":"cpp","protocol":"kattis"}`},
		{"mixed-answer-sources", EntryTest, `{"schemaVersion":1,"name":"t","input":{"kind":"file"},"answer":{"kind":"file","entry":"a","solution":"s"}}`},
		{"mixed-input-sources", EntryTest, `{"schemaVersion":1,"name":"t","input":{"kind":"generator","entry":"a"},"answer":{"kind":"solution"}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeMaterial(test.kind, []byte(test.data)); err == nil {
				t.Fatal("invalid material accepted")
			}
		})
	}
	// Incomplete links are valid drafts, to be diagnosed by package checking.
	if _, err := NormalizeMaterial(EntryTest, []byte(`{"schemaVersion":1,"name":"draft","input":{"kind":"generator"},"answer":{"kind":"solution"}}`)); err != nil {
		t.Fatalf("cannot save incomplete draft: %v", err)
	}
}
