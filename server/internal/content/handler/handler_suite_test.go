package handler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContentHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Content Handler Suite")
}
