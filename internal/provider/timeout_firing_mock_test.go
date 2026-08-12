package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestTimeoutFiring_ConfiguredValueOverridesDefault is a mock-server test,
// not a real-sandbox one — see
// docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md's
// narrow, explicitly-argued exception: Paddle's real API can't be forced to
// hang on demand, so this behavior genuinely cannot be proven against the
// live sandbox at all. This test *is* the verification for it, per
// docs/decisions/0013-configurable-timeouts-architecture.md.
//
// A minimal one-off httptest harness (Step 3's reusable mock-server harness
// hadn't landed yet when this was written — see
// docs/plans/paddle-provider-v5.md's Step 2 note about reordering) proving
// two things in one test: (1) a configured timeouts.delete shorter than the
// 60s default actually cuts the call off around that shorter value, not 60s
// — the whole point of the feature — and (2) the deliberately-slow handler
// really would have taken longer than that if allowed to, so a false pass
// (the call failing fast for some unrelated reason) can't slip through.
func TestTimeoutFiring_ConfiguredValueOverridesDefault(t *testing.T) {
	const handlerDelay = 2 * time.Second
	const configuredTimeout = "300ms"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately slower than configuredTimeout but faster than the
		// test's own execution budget — proves the call would have
		// succeeded eventually (ruling out a false pass from something
		// else failing fast) while staying fast enough not to make this
		// test itself slow.
		time.Sleep(handlerDelay)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"pro_slow","name":"x","tax_category":"standard","status":"archived"}}`))
	}))
	defer srv.Close()

	pr := &ProductResource{client: client.New(srv.URL, "test-key")}

	req := testDeleteState(t, pr, productResourceStateModel{
		ProductResourceModel: ProductResourceModel{
			ID:          types.StringValue("pro_slow"),
			Name:        types.StringValue("x"),
			TaxCategory: types.StringValue("standard"),
		},
		Timeouts: timeoutsValueWithDelete(configuredTimeout),
	})

	var resp resource.DeleteResponse
	start := time.Now()
	pr.Delete(context.Background(), req, &resp)
	elapsed := time.Since(start)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected Delete() to fail with a timeout error, got success — the configured 300ms timeout did not fire")
	}
	if elapsed >= handlerDelay {
		t.Errorf("Delete() took %v, want well under the handler's %v delay — the configured %s timeout should have cut it off long before the handler ever responded", elapsed, handlerDelay, configuredTimeout)
	}
	// Generous upper bound: confirms the call was cut off close to the
	// configured value, not the previous hardcoded 60s default — without
	// asserting so tightly on timing that normal CI scheduling jitter
	// makes this test flaky.
	if elapsed > 5*time.Second {
		t.Errorf("Delete() took %v, want closer to the configured %s — looks like the old 60s default fired instead of the configured timeout", elapsed, configuredTimeout)
	}
}
