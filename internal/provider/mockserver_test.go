package provider

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// mockProviderConfig is the provider block every mock-based test's Config
// starts with — deliberately empty (unlike provider_test.go's
// providerConfig, which pins environment = "sandbox"): PADDLE_BASE_URL
// (see provider.go's Configure()) overrides whatever environment resolves
// to, so there's nothing to pin here, and api_key/environment both come
// from the env vars newMockPaddleServer sets.
const mockProviderConfig = `
provider "paddle" {}
`

// newMockPaddleServer stands up an httptest.Server running handler and
// points this provider's client at it for the calling test's lifetime, via
// PADDLE_BASE_URL — provider.go's deliberately undocumented, internal-only
// escape hatch that exists specifically for this harness. t.Setenv() ties
// the override to the test (and any of its t.Run subtests), automatically
// restored after the test completes, and — same as any t.Setenv() use —
// makes the calling test incompatible with t.Parallel().
//
// Returns provider factories wired the same shape
// testAccProtoV6ProviderFactories (provider_test.go) uses, so a mock test
// drops straight into resource.Test exactly like a real acceptance test
// (see internal/provider/product_resource_acc_test.go for the pattern),
// just pointed at handler instead of Paddle's real sandbox — with
// IsUnitTest: true set in the TestCase (not here — that's a TestCase
// field, not something this helper can set), so it runs under plain `go
// test ./...` without TF_ACC or real credentials.
//
// docs/guardrails/mock-tests-supplement-not-replace-acceptance-tests.md:
// this harness is additive, a faster/cheaper signal underneath real-sandbox
// verification, never a replacement for it — every resource retrofitted
// with this harness keeps its existing *_resource_acc_test.go file
// completely unchanged, and mock-based test files are named
// *_resource_mock_test.go specifically so which kind of test a file
// contains is obvious from the filename alone, never ambiguous with the
// existing *_resource_acc_test.go real-sandbox files.
func newMockPaddleServer(t *testing.T, handler http.Handler) map[string]func() (tfprotov6.ProviderServer, error) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	t.Setenv("PADDLE_BASE_URL", srv.URL)
	t.Setenv("PADDLE_API_KEY", "mock-key")

	return map[string]func() (tfprotov6.ProviderServer, error){
		"paddle": providerserver.NewProtocol6WithError(New("acctest")()),
	}
}
