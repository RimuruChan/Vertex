package submission_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSubmission(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Submission Suite")
}
