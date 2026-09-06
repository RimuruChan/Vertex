package domain_test

import (
	"context"

	"github.com/RimuruChan/Vertex/server/internal/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domain persistence context", func() {
	It("uses official for legacy calls and never replaces an explicitly bound scope", func() {
		ctx := context.Background()
		Expect(domain.ID(ctx)).To(Equal(domain.OfficialID))
		_, ok := domain.FromContext(ctx)
		Expect(ok).To(BeFalse())
		scope := domain.Scope{Domain: domain.Domain{ID: "bound-domain"}, UserID: "viewer"}
		bound := domain.WithScope(ctx, scope)
		Expect(domain.ID(bound)).To(Equal("bound-domain"))
		value, ok := domain.FromContext(bound)
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal(scope))
		Expect(domain.ID(domain.WithScope(ctx, domain.Scope{}))).To(BeEmpty())
	})
})
