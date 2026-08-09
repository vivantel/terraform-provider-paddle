package provider

import (
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestFromAPICheckoutDomain_PopulatesNestedApplePayStatus(t *testing.T) {
	var m CheckoutDomainDataSourceModel
	fromAPICheckoutDomain(client.CheckoutDomain{
		ID:     "chedom_1",
		Domain: "checkout.example.com",
		Status: "approved",
		PaymentMethodVerification: client.PaymentMethodVerification{
			ApplePay: client.ApplePayVerification{Status: "verified"},
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-02T00:00:00Z",
	}, &m)

	if m.Domain.ValueString() != "checkout.example.com" {
		t.Errorf("Domain = %q, want checkout.example.com", m.Domain.ValueString())
	}
	if m.Status.ValueString() != "approved" {
		t.Errorf("Status = %q, want approved", m.Status.ValueString())
	}
	if m.PaymentMethodVerification.ApplePay.Status.ValueString() != "verified" {
		t.Errorf("PaymentMethodVerification.ApplePay.Status = %q, want verified", m.PaymentMethodVerification.ApplePay.Status.ValueString())
	}
}
