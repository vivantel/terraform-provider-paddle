package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// testAccCheckEventsContainsProduct asserts the *data source's own state*
// contains an event whose data payload mentions productID — checking
// state directly, not a second, independent client.ListEvents call the
// way an earlier version of this test did. That earlier version's
// PostApplyFunc queried the API directly and would report success even
// if EventsDataSource.Read itself were broken (e.g. the type filter
// mis-wired, or events never actually populated into state) — found via
// code review, fixed here so a real Read() bug fails this test.
func testAccCheckEventsContainsProduct(dataSourceName, productID string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("data source not found in state: %s", dataSourceName)
		}
		countStr, ok := rs.Primary.Attributes["events.#"]
		if !ok {
			return fmt.Errorf("events.# not set in state for %s", dataSourceName)
		}
		count, err := strconv.Atoi(countStr)
		if err != nil {
			return fmt.Errorf("events.# = %q: %w", countStr, err)
		}
		for i := 0; i < count; i++ {
			data := rs.Primary.Attributes[fmt.Sprintf("events.%d.data", i)]
			if strings.Contains(data, productID) {
				return nil
			}
		}
		return fmt.Errorf("none of the %d event(s) in %s's state contain product %s in their data field — either the type filter or Read() itself is broken", count, dataSourceName, productID)
	}
}

// TestAccPaddleEventsDataSource_productCreated needs no dedicated
// fixture, per docs/plans/paddle-provider-v4.md Step 5: creating a
// product (any acceptance test's setup already does this) generates a
// real product.created event, queryable by type immediately after.
// Archives the fixture product itself so it doesn't leak — the event
// this test is actually checking for stays in Paddle's own 90-day
// retention regardless of what happens to the product afterward.
func TestAccPaddleEventsDataSource_productCreated(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()
	suffix := randAccTestSuffix()
	name := "Acc Test Events DS Fixture " + suffix

	prod, err := c.CreateProduct(ctx, client.Product{Name: name, TaxCategory: "standard"})
	if err != nil {
		t.Fatalf("fixture CreateProduct: %v", err)
	}
	t.Cleanup(func() {
		if err := c.ArchiveProduct(context.Background(), prod.ID); err != nil && !client.IsNotFound(err) {
			t.Logf("cleanup: ArchiveProduct(%s): %v", prod.ID, err)
		}
	})

	dataSourceName := "data.paddle_events.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
data "paddle_events" "test" {
  type = ["product.created"]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "events.#"),
					testAccCheckEventsContainsProduct(dataSourceName, prod.ID),
				),
			},
		},
	})
}
