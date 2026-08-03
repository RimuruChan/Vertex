package handler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestJudgeHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Judge Handler Suite")
}
