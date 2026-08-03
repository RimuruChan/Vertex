package middleware

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHTTPMiddleware(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Middleware Suite")
}
