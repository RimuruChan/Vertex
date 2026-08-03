package problem_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProblem(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Problem Suite")
}
