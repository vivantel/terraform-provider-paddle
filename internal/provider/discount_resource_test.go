package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func baseDiscountModel() DiscountResourceModel {
	return DiscountResourceModel{
		Description:               types.StringValue("10% off"),
		Type:                      types.StringValue("percentage"),
		Amount:                    types.StringValue("10"),
		Code:                      types.StringNull(),
		EnabledForCheckout:        types.BoolValue(true),
		Mode:                      types.StringValue("standard"),
		CurrencyCode:              types.StringNull(),
		Recur:                     types.BoolValue(false),
		MaximumRecurringIntervals: types.Int64Null(),
		UsageLimit:                types.Int64Null(),
		RestrictTo:                types.ListNull(types.StringType),
		ExpiresAt:                 types.StringNull(),
		DiscountGroupID:           types.StringNull(),
	}
}

func TestToAPIDiscount_ClearingOptionalFieldsProducesNilPointer(t *testing.T) {
	m := baseDiscountModel()

	d, diags := toAPIDiscount(context.Background(), m)
	if diags.HasError() {
		t.Fatalf("toAPIDiscount: %v", diags)
	}

	if d.Code != nil {
		t.Errorf("Code = %q, want nil", *d.Code)
	}
	if d.CurrencyCode != nil {
		t.Errorf("CurrencyCode = %q, want nil", *d.CurrencyCode)
	}
	if d.MaximumRecurringIntervals != nil {
		t.Errorf("MaximumRecurringIntervals = %d, want nil", *d.MaximumRecurringIntervals)
	}
	if d.RestrictTo != nil {
		t.Errorf("RestrictTo = %v, want nil", d.RestrictTo)
	}
}

func TestToAPIDiscount_SetOptionalFieldsCarryThrough(t *testing.T) {
	m := baseDiscountModel()
	m.Code = types.StringValue("SAVE10")
	m.CurrencyCode = types.StringValue("USD")
	maxIntervals := types.Int64Value(3)
	m.MaximumRecurringIntervals = maxIntervals
	restrictTo, diags := types.ListValueFrom(context.Background(), types.StringType, []string{"pro_1", "pro_2"})
	if diags.HasError() {
		t.Fatalf("building restrict_to fixture: %v", diags)
	}
	m.RestrictTo = restrictTo

	d, diags := toAPIDiscount(context.Background(), m)
	if diags.HasError() {
		t.Fatalf("toAPIDiscount: %v", diags)
	}

	if d.Code == nil || *d.Code != "SAVE10" {
		t.Errorf("Code = %v, want SAVE10", d.Code)
	}
	if d.CurrencyCode == nil || *d.CurrencyCode != "USD" {
		t.Errorf("CurrencyCode = %v, want USD", d.CurrencyCode)
	}
	if d.MaximumRecurringIntervals == nil || *d.MaximumRecurringIntervals != 3 {
		t.Errorf("MaximumRecurringIntervals = %v, want 3", d.MaximumRecurringIntervals)
	}
	if len(d.RestrictTo) != 2 || d.RestrictTo[0] != "pro_1" || d.RestrictTo[1] != "pro_2" {
		t.Errorf("RestrictTo = %v, want [pro_1 pro_2]", d.RestrictTo)
	}
}

func TestToAPIDiscount_UnknownDefaultedFieldsOmitted(t *testing.T) {
	// Regression coverage for the same class of bug the price quantity
	// Default fix addressed: on Create, an Optional+Computed field left
	// unset in config is Unknown until the schema Default resolves it —
	// toAPIDiscount must not call ValueBool()/etc. on an Unknown value.
	m := baseDiscountModel()
	m.EnabledForCheckout = types.BoolUnknown()
	m.Mode = types.StringUnknown()
	m.Recur = types.BoolUnknown()
	// code is also Optional+Computed (Paddle auto-generates it — see the
	// schema comment) and was the one actually missed: real sandbox run
	// 31278240955 sent code: "" (ValueString() on an Unknown value
	// silently returns the zero value) instead of omitting the field,
	// which Paddle rejected outright. IsNull() alone is false for Unknown
	// too, so the fix needed an explicit IsUnknown() check here as well.
	m.Code = types.StringUnknown()

	d, diags := toAPIDiscount(context.Background(), m)
	if diags.HasError() {
		t.Fatalf("toAPIDiscount: %v", diags)
	}

	if d.EnabledForCheckout != nil {
		t.Errorf("EnabledForCheckout = %v, want nil (Unknown should be skipped)", d.EnabledForCheckout)
	}
	if d.Mode != "" {
		t.Errorf("Mode = %q, want empty (Unknown should be skipped)", d.Mode)
	}
	if d.Recur != nil {
		t.Errorf("Recur = %v, want nil (Unknown should be skipped)", d.Recur)
	}
	if d.Code != nil {
		t.Errorf("Code = %q, want nil (Unknown should be skipped, not sent as empty string)", *d.Code)
	}
}

func TestFromAPIDiscount_NilOptionalFieldsBecomeNull(t *testing.T) {
	var m DiscountResourceModel
	diags := fromAPIDiscount(context.Background(), client.Discount{
		ID:          "dsc_1",
		Description: "10% off",
		Type:        "percentage",
		Amount:      "10",
		Mode:        "standard",
		Status:      "active",
	}, &m)
	if diags.HasError() {
		t.Fatalf("fromAPIDiscount: %v", diags)
	}

	if !m.Code.IsNull() {
		t.Errorf("Code = %v, want null", m.Code)
	}
	if !m.CurrencyCode.IsNull() {
		t.Errorf("CurrencyCode = %v, want null", m.CurrencyCode)
	}
	if !m.RestrictTo.IsNull() {
		t.Errorf("RestrictTo = %v, want null", m.RestrictTo)
	}
}

func TestFromAPIDiscount_SetFieldsRoundTrip(t *testing.T) {
	code := "SAVE10"
	currency := "USD"
	restrictTo := []string{"pro_1"}

	var m DiscountResourceModel
	diags := fromAPIDiscount(context.Background(), client.Discount{
		ID:           "dsc_1",
		Description:  "10% off",
		Type:         "percentage",
		Amount:       "10",
		Code:         &code,
		CurrencyCode: &currency,
		RestrictTo:   restrictTo,
		Mode:         "standard",
		Status:       "active",
		TimesUsed:    5,
	}, &m)
	if diags.HasError() {
		t.Fatalf("fromAPIDiscount: %v", diags)
	}

	if m.Code.ValueString() != "SAVE10" {
		t.Errorf("Code = %q, want SAVE10", m.Code.ValueString())
	}
	if m.TimesUsed.ValueInt64() != 5 {
		t.Errorf("TimesUsed = %d, want 5", m.TimesUsed.ValueInt64())
	}
	var gotRestrictTo []string
	diags = m.RestrictTo.ElementsAs(context.Background(), &gotRestrictTo, false)
	if diags.HasError() {
		t.Fatalf("reading back RestrictTo: %v", diags)
	}
	if len(gotRestrictTo) != 1 || gotRestrictTo[0] != "pro_1" {
		t.Errorf("RestrictTo = %v, want [pro_1]", gotRestrictTo)
	}
}
