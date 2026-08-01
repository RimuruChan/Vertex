package store

import (
	"testing"
)

func TestSortRankRowsACM(t *testing.T) {
	rows := []RankRow{
		{Username: "bob", Solved: 3, Penalty: 300},
		{Username: "alice", Solved: 5, Penalty: 100},
		{Username: "carol", Solved: 3, Penalty: 200},
		{Username: "dave", Solved: 0, Penalty: 0},
	}
	sortRankRows(rows)

	want := []struct {
		name    string
		solved  int
		penalty int
	}{
		{"alice", 5, 100},
		{"carol", 3, 200}, // 同 solved,罚时少在前
		{"bob", 3, 300},
		{"dave", 0, 0},
	}
	for i, w := range want {
		if rows[i].Username != w.name {
			t.Errorf("rank %d = %s, want %s", i, rows[i].Username, w.name)
		}
	}
}

func TestSortRankRowsTieBreak(t *testing.T) {
	// 完全同分按用户名升序(稳定并列)
	rows := []RankRow{
		{Username: "zed", Solved: 2, Penalty: 100},
		{Username: "amy", Solved: 2, Penalty: 100},
	}
	sortRankRows(rows)
	if rows[0].Username != "amy" {
		t.Errorf("tie-break first = %s, want amy", rows[0].Username)
	}
}
