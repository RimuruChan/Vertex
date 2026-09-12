package domain_test

import (
	contestdomain "github.com/RimuruChan/Vertex/server/internal/modules/contest/domain"
	"reflect"
	"testing"
)

func TestMedalAllocation(t *testing.T) {
	rows := []contestdomain.RankRow{{Rank: 1}, {Rank: 2, Solved: 1}, {Rank: 2, Solved: 2}, {Rank: 4, Solved: 1}, {Rank: 5, Solved: 1}, {Rank: 6}, {Rank: 7, Solved: 1}}
	summary := contestdomain.AssignMedals(rows, contestdomain.MedalConfig{Mode: "count", Gold: 1, Silver: 2, Bronze: 1})
	got := []string{}
	for _, row := range rows {
		got = append(got, row.Medal)
	}
	if !reflect.DeepEqual(got, []string{"", "gold", "gold", "silver", "bronze", "", ""}) {
		t.Fatalf("medals = %v", got)
	}
	if summary.Eligible != 5 || summary.Gold != 2 || summary.Silver != 1 || summary.Bronze != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if contestdomain.AssignMedals(rows, contestdomain.MedalConfig{Mode: "none"}) != nil {
		t.Fatal("disabled awards must have no summary")
	}
	for _, row := range rows {
		if row.Medal != "" {
			t.Fatal("disabled awards left an old medal")
		}
	}
}

func TestMedalPercentages(t *testing.T) {
	rows := make([]contestdomain.RankRow, 10)
	for i := range rows {
		rows[i].Rank = i + 1
		if i < 7 {
			rows[i].Solved = 1
		}
	}
	summary := contestdomain.AssignMedals(rows, contestdomain.MedalConfig{Mode: "percentage", Gold: 10, Silver: 30, Bronze: 40})
	if summary.Eligible != 7 || summary.Gold != 0 || summary.Silver != 2 || summary.Bronze != 2 {
		t.Fatalf("individually rounded quotas = %+v", summary)
	}
	empty := contestdomain.AssignMedals([]contestdomain.RankRow{{Rank: 1}}, contestdomain.MedalConfig{Mode: "count", Gold: 10})
	if empty.Eligible != 0 || empty.Gold != 0 {
		t.Fatalf("zero-solve row got a medal: %+v", empty)
	}
	tied := []contestdomain.RankRow{{Rank: 1, Solved: 1}, {Rank: 1, Solved: 1}, {Rank: 3, Solved: 1}}
	contestdomain.AssignMedals(tied, contestdomain.MedalConfig{Mode: "count", Bronze: 1})
	if tied[0].Medal != "bronze" || tied[1].Medal != "bronze" || tied[2].Medal != "" {
		t.Fatalf("zero quotas/tie overflow: %+v", tied)
	}
}

func TestMedalValidation(t *testing.T) {
	for _, c := range []contestdomain.MedalConfig{{Mode: "bad"}, {Mode: "count", Gold: -1}, {Mode: "count", Bronze: 100001}, {Mode: "percentage", Gold: 51, Silver: 50}} {
		if c.Validate() == nil {
			t.Fatalf("accepted invalid config %+v", c)
		}
	}
	for _, c := range []contestdomain.MedalConfig{{Mode: "none"}, {Mode: "count", Gold: 10}, {Mode: "percentage", Gold: 20, Silver: 30, Bronze: 50}} {
		if err := c.Validate(); err != nil {
			t.Fatalf("valid config %+v: %v", c, err)
		}
	}
}

func TestDefaultMedals(t *testing.T) {
	for _, format := range []string{"", "icpc", "oi", "ioi", "cf", "leduo"} {
		got := contestdomain.DefaultMedals(format)
		want := contestdomain.MedalConfig{Mode: "none"}
		if format == "" || format == "icpc" {
			want = contestdomain.MedalConfig{Mode: "percentage", Gold: 10, Silver: 20, Bronze: 30}
		}
		if got != want {
			t.Fatalf("%q default medals: %+v, want %+v", format, got, want)
		}
	}
}
