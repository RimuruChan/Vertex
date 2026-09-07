package contest_test

import (
	"time"

	"github.com/RimuruChan/Vertex/server/internal/contest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Registration window", func() {
	begin := time.Unix(1_800_000_000, 0)
	end := begin.Add(time.Hour)
	DescribeTable("combines both switches with an exclusive end boundary",
		func(self, late bool, now time.Time, open bool) {
			item := contest.Contest{BeginAt: begin, EndAt: end, AllowSelfRegistration: self, AllowLateRegistration: late}
			Expect(item.RegistrationOpen(now)).To(Equal(open))
		},
		Entry("default before start", true, false, begin.Add(-time.Nanosecond), true),
		Entry("default at start", true, false, begin, false),
		Entry("default during contest", true, false, begin.Add(time.Minute), false),
		Entry("late before start", true, true, begin.Add(-time.Nanosecond), true),
		Entry("late at start", true, true, begin, true),
		Entry("late just before end", true, true, end.Add(-time.Nanosecond), true),
		Entry("late at end", true, true, end, false),
		Entry("late after end", true, true, end.Add(time.Nanosecond), false),
		Entry("disabled before start", false, false, begin.Add(-time.Minute), false),
		Entry("late cannot override disabled", false, true, begin.Add(time.Minute), false),
	)
})
