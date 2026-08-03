package handler

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIdentityHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Identity Handler Suite")
}
