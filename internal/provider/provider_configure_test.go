package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// newProviderConfig builds a real tfsdk.Config against the provider's own
// schema, the same shape Terraform core would send — used to exercise
// Configure() directly without the full acceptance-test machinery, since
// these are pure request/response unit tests.
func newProviderConfig(t *testing.T, apiKey, environment tftypes.Value) provider.ConfigureRequest {
	t.Helper()
	p := &PaddleProvider{}
	var schemaResp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &schemaResp)

	objType := tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"api_key":     tftypes.String,
			"environment": tftypes.String,
		},
	}
	raw := tftypes.NewValue(objType, map[string]tftypes.Value{
		"api_key":     apiKey,
		"environment": environment,
	})

	return provider.ConfigureRequest{
		Config: tfsdk.Config{Raw: raw, Schema: schemaResp.Schema},
	}
}

func TestConfigure_UnknownAPIKeyDoesNotClobberEnvFallback(t *testing.T) {
	// Regression test for /code-review high finding: Configure() checked
	// only config.APIKey.IsNull(), not IsUnknown() too. IsNull() is false
	// for an Unknown value (e.g. api_key set to a not-yet-applied resource
	// attribute) just as it is for a Known one, so ValueString() on the
	// Unknown value silently returned "" and overwrote a perfectly valid
	// PADDLE_API_KEY env var, producing a spurious "Missing Paddle API
	// key" error diagnostic.
	t.Setenv("PADDLE_API_KEY", "env-key")
	t.Setenv("PADDLE_ENVIRONMENT", "")

	req := newProviderConfig(t,
		tftypes.NewValue(tftypes.String, tftypes.UnknownValue), // api_key: Unknown
		tftypes.NewValue(tftypes.String, nil),                  // environment: Null
	)

	p := &PaddleProvider{}
	var resp provider.ConfigureResponse
	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure() produced errors with an Unknown api_key and a valid PADDLE_API_KEY env var set: %v", resp.Diagnostics)
	}
	c, ok := resp.ResourceData.(*client.Client)
	if !ok {
		t.Fatalf("ResourceData type = %T, want *client.Client", resp.ResourceData)
	}
	if c.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want %q (env fallback must not be clobbered by an Unknown config value)", c.APIKey, "env-key")
	}
}

func TestConfigure_UnknownEnvironmentDoesNotClobberEnvFallback(t *testing.T) {
	// Same bug class as above, for the environment attribute.
	t.Setenv("PADDLE_API_KEY", "")
	t.Setenv("PADDLE_ENVIRONMENT", "production")

	req := newProviderConfig(t,
		tftypes.NewValue(tftypes.String, "a-key"),
		tftypes.NewValue(tftypes.String, tftypes.UnknownValue), // environment: Unknown
	)

	p := &PaddleProvider{}
	var resp provider.ConfigureResponse
	p.Configure(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure() produced errors with an Unknown environment and a valid PADDLE_ENVIRONMENT env var set: %v", resp.Diagnostics)
	}
	c, ok := resp.ResourceData.(*client.Client)
	if !ok {
		t.Fatalf("ResourceData type = %T, want *client.Client", resp.ResourceData)
	}
	if c.BaseURL != client.ProductionBaseURL {
		t.Errorf("BaseURL = %q, want %q (PADDLE_ENVIRONMENT=production fallback must not be clobbered by an Unknown config value)", c.BaseURL, client.ProductionBaseURL)
	}
}
