package provider

import (
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestFromAPICustomer(t *testing.T) {
	cust := client.Customer{
		ID:     "ctm_01abc",
		Email:  "test@example.invalid",
		Name:   "Test Customer",
		Status: "active",
	}
	var m CustomerDataSourceModel
	fromAPICustomer(cust, &m)

	if m.ID.ValueString() != "ctm_01abc" {
		t.Errorf("ID = %q, want ctm_01abc", m.ID.ValueString())
	}
	if m.Email.ValueString() != "test@example.invalid" {
		t.Errorf("Email = %q, want test@example.invalid", m.Email.ValueString())
	}
	if m.Name.ValueString() != "Test Customer" {
		t.Errorf("Name = %q, want Test Customer", m.Name.ValueString())
	}
	if m.Status.ValueString() != "active" {
		t.Errorf("Status = %q, want active", m.Status.ValueString())
	}
}
