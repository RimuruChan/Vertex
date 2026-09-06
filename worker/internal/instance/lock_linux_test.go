//go:build linux

package instance_test

import (
	"path/filepath"

	"github.com/RimuruChan/Vertex/worker/internal/instance"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("sandbox instance lock", func() {
	It("rejects a duplicate holder and can be reacquired after release", func() {
		path := filepath.Join(GinkgoT().TempDir(), "instance.lock")
		first, err := instance.Acquire(path)
		Expect(err).NotTo(HaveOccurred())

		_, err = instance.Acquire(path)
		Expect(err).To(MatchError(ContainSubstring("already active")))

		Expect(first.Close()).To(Succeed())
		second, err := instance.Acquire(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Close()).To(Succeed())
	})
})
