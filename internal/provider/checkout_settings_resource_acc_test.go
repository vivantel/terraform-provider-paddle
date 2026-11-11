package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleCheckoutSettings_basic changes the sandbox account's real
// default payment link, which other acceptance tests depend on
// (paddle_adjustment needs one set to create any transaction). It records
// the value found beforehand and restores it in CheckDestroy, so a run
// leaves the account as it found it; destroy alone would clear it.
func TestAccPaddleCheckoutSettings_basic(t *testing.T) {
	resourceName := "paddle_checkout_settings.test"
	var original *string

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			got, err := newTestAccClient().GetCheckoutURLSettings(context.Background())
			if err != nil {
				t.Fatalf("reading existing checkout URL: %v", err)
			}
			original = got.DefaultURL
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(_ *terraform.State) error {
			c := newTestAccClient()
			got, err := c.GetCheckoutURLSettings(context.Background())
			if err != nil {
				return err
			}
			if got.DefaultURL != nil {
				t.Errorf("default_url = %q after destroy, want null", *got.DefaultURL)
			}
			// Put back what was there before this test touched it.
			_, err = c.UpdateCheckoutURLSettings(context.Background(), client.CheckoutURLSettingsUpdate{DefaultURL: original})
			return err
		},
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccCheckoutSettingsConfig("https://example.com/acc-test"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "id", "checkout-url"),
					resource.TestCheckResourceAttr(resourceName, "default_url", "https://example.com/acc-test"),
				),
			},
			{
				Config: providerConfig + testAccCheckoutSettingsConfig("https://example.com/acc-test-2"),
				Check:  resource.TestCheckResourceAttr(resourceName, "default_url", "https://example.com/acc-test-2"),
			},
			{
				Config:   providerConfig + testAccCheckoutSettingsConfig("https://example.com/acc-test-2"),
				PlanOnly: true,
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateId:           "checkout-url",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}

func testAccCheckoutSettingsConfig(url string) string {
	return `
resource "paddle_checkout_settings" "test" {
  default_url = "` + url + `"
}
`
}
