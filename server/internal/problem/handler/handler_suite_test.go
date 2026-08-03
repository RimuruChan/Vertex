package handler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestProblemHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Problem Handler Suite")
}
