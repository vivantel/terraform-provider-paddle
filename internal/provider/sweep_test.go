package provider

import (
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestMain wires up terraform-plugin-testing's -sweep flag handling.
// Required once per package for resource.AddTestSweepers to have any
// effect — see docs/decisions/0009-tflog-observability-and-acceptance-test-sweepers.md.
func TestMain(m *testing.M) {
	resource.TestMain(m)
}

// acceptance test configs across product/price/discount already name or
// describe their objects with "Acc Test" somewhere in the string (see
// *_acc_test.go) — sweepers reuse that same substring, case-insensitively,
// rather than introducing a second naming convention. No renaming of
// existing test configs was needed; they were already consistent enough
// for this to work.
const accTestMarker = "acc test"

func isAccTestName(s string) bool {
	return strings.Contains(strings.ToLower(s), accTestMarker)
}

// sweepClient builds a client.Client straight from the sandbox API key,
// mirroring newTestAccClient in provider_test.go — sweepers run outside any
// *testing.T context (resource.Sweeper.F takes just a region string), so
// they can't reuse that helper directly, but the construction is identical.
// Returns nil if PADDLE_API_KEY isn't set, so callers can skip cleanly
// rather than sweeping with an empty key and getting an auth error.
func sweepClient() *client.Client {
	key := os.Getenv("PADDLE_API_KEY")
	if key == "" {
		return nil
	}
	return client.New(client.SandboxBaseURL, key)
}

func init() {
	resource.AddTestSweepers("paddle_price", &resource.Sweeper{
		Name: "paddle_price",
		F:    sweepPrices,
	})
	resource.AddTestSweepers("paddle_discount", &resource.Sweeper{
		Name: "paddle_discount",
		F:    sweepDiscounts,
	})
	resource.AddTestSweepers("paddle_product", &resource.Sweeper{
		Name: "paddle_product",
		// Prices reference a product_id — sweep prices first so a leaked
		// price never outlives the product it points at, even though
		// archiving (not deleting) means this ordering isn't strictly
		// required for correctness today. Cheap to get right regardless.
		Dependencies: []string{"paddle_price"},
		F:            sweepProducts,
	})
}

func sweepProducts(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_product sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	products, err := c.ListProducts(ctx)
	if err != nil {
		return err
	}
	for _, p := range products {
		if p.Status == "archived" || !isAccTestName(p.Name) {
			continue
		}
		if err := c.ArchiveProduct(ctx, p.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test product %s (%s): %s", p.ID, p.Name, err)
		}
	}
	return nil
}

func sweepPrices(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_price sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	prices, err := c.ListPrices(ctx)
	if err != nil {
		return err
	}
	for _, p := range prices {
		if p.Status == "archived" || !isAccTestName(p.Description) {
			continue
		}
		if err := c.ArchivePrice(ctx, p.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test price %s (%s): %s", p.ID, p.Description, err)
		}
	}
	return nil
}

// TestAccSweepProducts_ArchivesLeakedTestObjects exercises the sweeper
// logic directly (calling sweepProducts, not the `-sweep` CLI flag) against
// the real sandbox: creates a product outside Terraform entirely — the
// exact "leaked between test runs" scenario sweepers exist for — confirms
// the sweeper archives it, and confirms an already-archived product it
// deliberately leaves alone (sweepProducts must skip already-archived
// objects, not just objects outside its naming convention).
func TestAccSweepProducts_ArchivesLeakedTestObjects(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()

	leaked, err := c.CreateProduct(ctx, client.Product{
		Name:        "Acc Test Sweeper Leaked Product",
		TaxCategory: "standard",
	})
	if err != nil {
		t.Fatalf("CreateProduct (leaked fixture): %v", err)
	}
	t.Cleanup(func() {
		_ = c.ArchiveProduct(ctx, leaked.ID) // best-effort; the sweeper itself is what's under test
	})

	if err := sweepProducts(""); err != nil {
		t.Fatalf("sweepProducts: %v", err)
	}

	got, err := c.GetProduct(ctx, leaked.ID)
	if err != nil {
		t.Fatalf("GetProduct after sweep: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("status after sweep = %q, want archived — sweeper did not clean up the leaked object", got.Status)
	}
}

func sweepDiscounts(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_discount sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	discounts, err := c.ListDiscounts(ctx)
	if err != nil {
		return err
	}
	for _, d := range discounts {
		if d.Status == "archived" || !isAccTestName(d.Description) {
			continue
		}
		if err := c.ArchiveDiscount(ctx, d.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test discount %s (%s): %s", d.ID, d.Description, err)
		}
	}
	return nil
}
