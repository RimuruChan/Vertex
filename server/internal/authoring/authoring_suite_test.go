package authoring

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAuthoring(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Authoring Suite")
}
