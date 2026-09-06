package problemset_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProblemSet(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Problem Set Suite")
}
