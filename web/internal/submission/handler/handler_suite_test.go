package handler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSubmissionHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Submission Handler Suite")
}
