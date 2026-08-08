package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func baseModel() PriceResourceModel {
	return PriceResourceModel{
		ProductID:   types.StringValue("pro_123"),
		Description: types.StringValue("internal desc"),
		UnitPrice: unitPriceModel{
			Amount:       types.StringValue("1000"),
			CurrencyCode: types.StringValue("USD"),
		},
	}
}

func TestToAPIPrice_QuantityOmittedUnlessBothBoundsKnown(t *testing.T) {
	tests := []struct {
		name     string
		quantity *quantityModel
		wantNil  bool
	}{
		{
			name:     "no quantity block at all",
			quantity: nil,
			wantNil:  true,
		},
		{
			name: "both bounds known",
			quantity: &quantityModel{
				Minimum: types.Int64Value(1),
				Maximum: types.Int64Value(100),
			},
			wantNil: false,
		},
		{
			name: "minimum unknown, maximum known",
			quantity: &quantityModel{
				Minimum: types.Int64Unknown(),
				Maximum: types.Int64Value(100),
			},
			wantNil: true,
		},
		{
			// Regression test: toAPIPrice used to check only Minimum's
			// unknown-ness. If Maximum were unknown while Minimum was
			// known, the old code would send Maximum as 0 instead of
			// leaving Quantity unset entirely.
			name: "maximum unknown, minimum known",
			quantity: &quantityModel{
				Minimum: types.Int64Value(1),
				Maximum: types.Int64Unknown(),
			},
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := baseModel()
			m.Quantity = tc.quantity

			got := toAPIPrice(m)

			if (got.Quantity == nil) != tc.wantNil {
				t.Fatalf("Quantity = %v, wantNil %v", got.Quantity, tc.wantNil)
			}
			if !tc.wantNil {
				if got.Quantity.Minimum != tc.quantity.Minimum.ValueInt64() {
					t.Errorf("Quantity.Minimum = %d, want %d", got.Quantity.Minimum, tc.quantity.Minimum.ValueInt64())
				}
				if got.Quantity.Maximum != tc.quantity.Maximum.ValueInt64() {
					t.Errorf("Quantity.Maximum = %d, want %d", got.Quantity.Maximum, tc.quantity.Maximum.ValueInt64())
				}
			}
		})
	}
}

func TestToAPIPrice_ClearingNameProducesNilPointer(t *testing.T) {
	m := baseModel()
	m.Name = types.StringNull()

	got := toAPIPrice(m)

	if got.Name != nil {
		t.Errorf("Name = %q, want nil", *got.Name)
	}
}

func TestFromAPIPrice_NilNameBecomesStringNull(t *testing.T) {
	var m PriceResourceModel
	fromAPIPrice(client.Price{
		ID:          "pri_1",
		ProductID:   "pro_123",
		Description: "internal desc",
		UnitPrice:   client.Money{Amount: "1000", CurrencyCode: "USD"},
	}, &m)

	if !m.Name.IsNull() {
		t.Errorf("Name = %v, want null", m.Name)
	}
}
