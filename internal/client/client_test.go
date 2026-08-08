package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// These test the wire format directly — no HTTP involved — because every
// bug /code-review high found in the archive/clear-field path was a JSON
// marshaling mistake, not a networking one. A live sandbox call wouldn't
// have caught "field silently omitted instead of nulled" any faster than
// json.Marshal does here.

func TestProductJSON_NilOptionalFieldsMarshalAsExplicitNull(t *testing.T) {
	desc := "widget description"
	img := "https://example.com/widget.png"

	tests := []struct {
		name string
		p    Product
		want string
	}{
		{
			name: "unset optional fields marshal as null, not omitted",
			p:    Product{Name: "Widget", TaxCategory: "standard"},
			want: `{"name":"Widget","tax_category":"standard","description":null,"image_url":null}`,
		},
		{
			name: "set optional fields marshal their value",
			p:    Product{Name: "Widget", TaxCategory: "standard", Description: &desc, ImageURL: &img},
			want: `{"name":"Widget","tax_category":"standard","description":"widget description","image_url":"https://example.com/widget.png"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, b, tc.want)
		})
	}
}

func TestPriceJSON_NilNameMarshalsAsExplicitNull(t *testing.T) {
	name := "Widget Monthly"

	tests := []struct {
		name string
		p    Price
		want string
	}{
		{
			name: "unset name marshals as null, not omitted",
			p: Price{
				ProductID:   "pro_123",
				Description: "internal desc",
				UnitPrice:   Money{Amount: "1000", CurrencyCode: "USD"},
			},
			want: `{"product_id":"pro_123","description":"internal desc","unit_price":{"amount":"1000","currency_code":"USD"},"name":null}`,
		},
		{
			name: "set name marshals its value",
			p: Price{
				ProductID:   "pro_123",
				Description: "internal desc",
				UnitPrice:   Money{Amount: "1000", CurrencyCode: "USD"},
				Name:        &name,
			},
			want: `{"product_id":"pro_123","description":"internal desc","unit_price":{"amount":"1000","currency_code":"USD"},"name":"Widget Monthly"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, b, tc.want)
		})
	}
}

func TestDiscountJSON_NilOptionalFieldsMarshalAsExplicitNull(t *testing.T) {
	code := "SAVE10"

	tests := []struct {
		name string
		d    Discount
		want string
	}{
		{
			name: "unset nullable fields marshal as null, not omitted",
			d:    Discount{Description: "10% off", Type: "percentage", Amount: "10"},
			want: `{"description":"10% off","type":"percentage","amount":"10",
				"code":null,"currency_code":null,"maximum_recurring_intervals":null,
				"usage_limit":null,"restrict_to":null,"expires_at":null,"discount_group_id":null}`,
		},
		{
			name: "set code marshals its value, other nullable fields stay null",
			d:    Discount{Description: "10% off", Type: "percentage", Amount: "10", Code: &code},
			want: `{"description":"10% off","type":"percentage","amount":"10",
				"code":"SAVE10","currency_code":null,"maximum_recurring_intervals":null,
				"usage_limit":null,"restrict_to":null,"expires_at":null,"discount_group_id":null}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.d)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, b, tc.want)
		})
	}
}

func TestDiscountJSON_ReadOnlyFieldsNeverSentInRequestBody(t *testing.T) {
	// A Discount built from a resource model (as toAPIDiscount in
	// discount_resource.go does) never sets TimesUsed/CreatedAt/UpdatedAt;
	// they're populated only by unmarshaling a response. Confirms they stay
	// omitted (not sent as 0/"") in that case.
	d := Discount{Description: "10% off", Type: "percentage", Amount: "10"}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, field := range []string{"times_used", "created_at", "updated_at"} {
		if strings.Contains(string(b), `"`+field+`"`) {
			t.Errorf("request body contains %q, want it entirely absent: %s", field, b)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	// Regression coverage for /code-review high findings: the 404 check
	// (errors.As + StatusCode == 404) was copy-pasted identically in all
	// three resources' Read(), using a magic 404, and was missing entirely
	// from Delete() in all three — this is the shared helper both should
	// use instead.
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "404 APIError", err: &APIError{StatusCode: 404}, want: true},
		{name: "wrapped 404 APIError", err: fmt.Errorf("read: %w", &APIError{StatusCode: 404}), want: true},
		{name: "non-404 APIError", err: &APIError{StatusCode: 400}, want: false},
		{name: "non-APIError", err: errors.New("boom"), want: false},
		{name: "nil error", err: nil, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNotFound(tc.err); got != tc.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestStatusPatchJSON_OnlyStatusField(t *testing.T) {
	// Regression test for the archive bug: ArchiveProduct/ArchivePrice used
	// to build a zero-value Product/Price with just Status set, which
	// serialized required fields (name, tax_category, product_id,
	// description, unit_price) as empty strings since those lack
	// omitempty. statusPatch must be the only thing that PATCH body sends.
	b, err := json.Marshal(statusPatch{Status: "archived"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"status":"archived"}`)
}

// assertJSONEqual compares two JSON documents structurally (key order in
// the "want" literal is just for readability, not semantics).
func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotVal, wantVal any
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("got is not valid JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("want is not valid JSON: %v\n%s", err, want)
	}
	gotNorm, _ := json.Marshal(gotVal)
	wantNorm, _ := json.Marshal(wantVal)
	if string(gotNorm) != string(wantNorm) {
		t.Errorf("JSON mismatch:\n got:  %s\n want: %s", got, want)
	}
}
