package provider

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// providerConfig is prepended to every acceptance test's Config. environment
// is pinned to sandbox explicitly rather than relying on the provider's
// sandbox-default (docs/decisions/0002-provider-auth-schema-with-env-fallback.md)
// — a test config should never depend on a default that could change.
const providerConfig = `
provider "paddle" {
  environment = "sandbox"
}
`

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"paddle": providerserver.NewProtocol6WithError(New("acctest")()),
}

// testAccPreCheck gates every acceptance test behind PADDLE_API_KEY being
// set, per docs/guardrails/acceptance-tests-require-tf-acc-gate.md — it
// must skip (not fail) so `go test ./...` stays usable without sandbox
// credentials, both locally and on fork PRs where CI secrets aren't
// injected.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("PADDLE_API_KEY") == "" {
		t.Skip("PADDLE_API_KEY not set — skipping acceptance test")
	}
}

// newTestAccClient builds a client.Client straight from the sandbox API
// key, for CheckDestroy assertions that need to inspect Paddle's actual
// state directly rather than through Terraform's state file. Takes no
// *testing.T since resource.TestCheckFunc's signature doesn't provide one.
func newTestAccClient() *client.Client {
	return client.New(client.SandboxBaseURL, os.Getenv("PADDLE_API_KEY"))
}

// randAccTestSuffix returns an 8-hex-character random suffix for
// acceptance test fixture names/descriptions that Paddle enforces
// uniqueness on (discovered for paddle_discount_group's `name` via a real
// sandbox 409 "discount_group_name_conflict" — two CI jobs for the same
// commit, `push` and `pull_request`, ran acceptance tests concurrently
// against the same sandbox account using the same fixed name). Most
// resources in this provider don't need this (Product/Price/Discount
// don't enforce name/description uniqueness), so this is opt-in per test,
// not baked into providerConfig or every *Config helper.
func randAccTestSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
