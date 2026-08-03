package handler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContestHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contest Handler Suite")
}
