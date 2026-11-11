package provider

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestToAPICheckoutSettingsUpdate_CarriesURL(t *testing.T) {
	got := toAPICheckoutSettingsUpdate(CheckoutSettingsResourceModel{DefaultURL: types.StringValue("https://example.com/buy")})
	if got.DefaultURL == nil || *got.DefaultURL != "https://example.com/buy" {
		t.Fatalf("DefaultURL = %v, want https://example.com/buy", got.DefaultURL)
	}
}

func TestToAPICheckoutSettingsUpdate_NullAndUnknownSendNull(t *testing.T) {
	for name, v := range map[string]types.String{"null": types.StringNull(), "unknown": types.StringUnknown()} {
		got := toAPICheckoutSettingsUpdate(CheckoutSettingsResourceModel{DefaultURL: v})
		if got.DefaultURL != nil {
			t.Errorf("%s: DefaultURL = %v, want nil", name, *got.DefaultURL)
		}
	}
}

// The clear-on-destroy path depends on a nil URL marshaling as an explicit
// null, not being omitted.
func TestCheckoutURLSettingsUpdate_NilMarshalsAsExplicitNull(t *testing.T) {
	b, err := json.Marshal(client.CheckoutURLSettingsUpdate{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"default_url":null}` {
		t.Errorf("body = %s, want {\"default_url\":null}", b)
	}
}

func TestFromAPICheckoutSettings_PopulatesModel(t *testing.T) {
	u := "https://example.com/buy"
	var m CheckoutSettingsResourceModel
	fromAPICheckoutSettings(client.CheckoutURLSettings{DefaultURL: &u}, &m)
	if m.ID.ValueString() != checkoutSettingsID {
		t.Errorf("ID = %q, want %q", m.ID.ValueString(), checkoutSettingsID)
	}
	if m.DefaultURL.ValueString() != u {
		t.Errorf("DefaultURL = %q, want %q", m.DefaultURL.ValueString(), u)
	}
}

func TestFromAPICheckoutSettings_NilURLIsNull(t *testing.T) {
	var m CheckoutSettingsResourceModel
	fromAPICheckoutSettings(client.CheckoutURLSettings{}, &m)
	if !m.DefaultURL.IsNull() {
		t.Errorf("DefaultURL = %v, want null", m.DefaultURL)
	}
}

func TestCheckoutURLPattern(t *testing.T) {
	for in, want := range map[string]bool{
		"https://example.com/buy": true,
		"http://example.com/buy":  false,
		"https://exa mple.com":    false,
		"https://":                false,
		"":                        false,
	} {
		if got := checkoutURLPattern.MatchString(in); got != want {
			t.Errorf("%q: match = %v, want %v", in, got, want)
		}
	}
}
