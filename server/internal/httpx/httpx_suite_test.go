package httpx

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHTTPX(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTPX Suite")
}
