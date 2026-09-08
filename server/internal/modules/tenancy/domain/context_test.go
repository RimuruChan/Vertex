package domain_test

import (
	"context"

	tenancydomain "github.com/RimuruChan/Vertex/server/internal/modules/tenancy/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domain persistence context", func() {
	It("uses official for legacy calls and never replaces an explicitly bound scope", func() {
		ctx := context.Background()
		Expect(tenancydomain.ID(ctx)).To(Equal(tenancydomain.OfficialID))
		_, ok := tenancydomain.FromContext(ctx)
		Expect(ok).To(BeFalse())
		scope := tenancydomain.Scope{Domain: tenancydomain.Domain{ID: "bound-domain"}, UserID: "viewer"}
		bound := tenancydomain.WithScope(ctx, scope)
		Expect(tenancydomain.ID(bound)).To(Equal("bound-domain"))
		value, ok := tenancydomain.FromContext(bound)
		Expect(ok).To(BeTrue())
		Expect(value).To(Equal(scope))
		Expect(tenancydomain.ID(tenancydomain.WithScope(ctx, tenancydomain.Scope{}))).To(BeEmpty())
	})
})
