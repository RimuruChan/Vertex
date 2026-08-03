package contest

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rank ordering", func() {
	It("sorts by solved count and then penalty", func() {
		rows := []RankRow{
			{Username: "bob", Solved: 3, Penalty: 300},
			{Username: "alice", Solved: 5, Penalty: 100},
			{Username: "carol", Solved: 3, Penalty: 200},
			{Username: "dave", Solved: 0, Penalty: 0},
		}
		sortRankRows(rows)
		Expect(rows).To(HaveExactElements(
			RankRow{Username: "alice", Solved: 5, Penalty: 100},
			RankRow{Username: "carol", Solved: 3, Penalty: 200},
			RankRow{Username: "bob", Solved: 3, Penalty: 300},
			RankRow{Username: "dave", Solved: 0, Penalty: 0},
		))
	})

	It("uses username as a deterministic final tie-break", func() {
		rows := []RankRow{
			{Username: "zed", Solved: 2, Penalty: 100},
			{Username: "amy", Solved: 2, Penalty: 100},
		}
		sortRankRows(rows)
		Expect(rows).To(HaveExactElements(
			RankRow{Username: "amy", Solved: 2, Penalty: 100},
			RankRow{Username: "zed", Solved: 2, Penalty: 100},
		))
	})
})
