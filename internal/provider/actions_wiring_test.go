package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// TestProviderServer_ExposesAllFiveActionSchemas exercises the real proto
// layer end to end — GetProviderSchema makes no network call (pure schema
// assembly), so this runs without PADDLE_API_KEY or TF_ACC, unlike this
// provider's TestAcc* suite. Confirms Actions() is actually wired into
// New() (not just present on PaddleProvider in isolation — see
// internal/provider/actions' own TestActions_MetadataAndSchemaBuildWithoutError
// for that half) and that the Plugin Framework's own action-schema
// conversion doesn't reject any of the five schemas.
func TestProviderServer_ExposesAllFiveActionSchemas(t *testing.T) {
	srv, err := testAccProtoV6ProviderFactories["paddle"]()
	if err != nil {
		t.Fatalf("provider factory: %v", err)
	}

	resp, err := srv.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatalf("GetProviderSchema: %v", err)
	}
	if len(resp.Diagnostics) > 0 {
		t.Fatalf("GetProviderSchema diagnostics: %+v", resp.Diagnostics)
	}

	want := []string{
		"paddle_adjustment",
		"paddle_subscription_cancel",
		"paddle_subscription_pause",
		"paddle_subscription_resume",
		"paddle_subscription_charge",
	}
	for _, name := range want {
		if _, ok := resp.ActionSchemas[name]; !ok {
			t.Errorf("ActionSchemas[%q] missing, want present. Got keys: %v", name, keysOf(resp.ActionSchemas))
		}
	}
	if len(resp.ActionSchemas) != len(want) {
		t.Errorf("len(ActionSchemas) = %d, want %d — an unexpected action is registered or one is missing", len(resp.ActionSchemas), len(want))
	}

	// Sanity check that adding Actions() didn't disturb the existing
	// resource/data source registration.
	if len(resp.ResourceSchemas) != 5 {
		t.Errorf("len(ResourceSchemas) = %d, want 5", len(resp.ResourceSchemas))
	}
	if len(resp.DataSourceSchemas) != 15 {
		t.Errorf("len(DataSourceSchemas) = %d, want 15", len(resp.DataSourceSchemas))
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
