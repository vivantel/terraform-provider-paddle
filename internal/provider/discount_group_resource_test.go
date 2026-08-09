package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestToAPIDiscountGroup_CarriesName(t *testing.T) {
	m := DiscountGroupResourceModel{Name: types.StringValue("VIP Customers")}
	got := toAPIDiscountGroup(m)
	if got.Name != "VIP Customers" {
		t.Errorf("Name = %q, want %q", got.Name, "VIP Customers")
	}
}

func TestFromAPIDiscountGroup_PopulatesModel(t *testing.T) {
	var m DiscountGroupResourceModel
	fromAPIDiscountGroup(client.DiscountGroup{ID: "dsg_1", Name: "VIP Customers", Status: "active"}, &m)

	if m.ID.ValueString() != "dsg_1" {
		t.Errorf("ID = %q, want dsg_1", m.ID.ValueString())
	}
	if m.Name.ValueString() != "VIP Customers" {
		t.Errorf("Name = %q, want VIP Customers", m.Name.ValueString())
	}
	if m.Status.ValueString() != "active" {
		t.Errorf("Status = %q, want active", m.Status.ValueString())
	}
}
