package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestToAPIProduct_ClearingOptionalFieldsProducesNilPointer(t *testing.T) {
	tests := []struct {
		name  string
		model ProductResourceModel
		want  *string // expected Description pointer value, nil meaning "clear"
	}{
		{
			name: "description null in config clears it",
			model: ProductResourceModel{
				Name:        types.StringValue("Widget"),
				TaxCategory: types.StringValue("standard"),
				Description: types.StringNull(),
			},
			want: nil,
		},
		{
			name: "description set in config carries the value",
			model: ProductResourceModel{
				Name:        types.StringValue("Widget"),
				TaxCategory: types.StringValue("standard"),
				Description: types.StringValue("a widget"),
			},
			want: strPtr("a widget"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := toAPIProduct(tc.model)
			if err != nil {
				t.Fatalf("toAPIProduct: %v", err)
			}
			if (got.Description == nil) != (tc.want == nil) {
				t.Fatalf("Description nil-ness mismatch: got %v, want %v", got.Description, tc.want)
			}
			if got.Description != nil && *got.Description != *tc.want {
				t.Errorf("Description = %q, want %q", *got.Description, *tc.want)
			}
		})
	}
}

func TestFromAPIProduct_NilDescriptionBecomesStringNull(t *testing.T) {
	var m ProductResourceModel
	if err := fromAPIProduct(client.Product{ID: "pro_1", Name: "Widget", TaxCategory: "standard"}, &m); err != nil {
		t.Fatalf("fromAPIProduct: %v", err)
	}

	if !m.Description.IsNull() {
		t.Errorf("Description = %v, want null", m.Description)
	}
}

func strPtr(s string) *string { return &s }
