package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestAccPaddleNotificationSetting_basic(t *testing.T) {
	resourceName := "paddle_notification_setting.test"
	// destination is opaque to Paddle for a url-type setting (no delivery
	// attempt is required to create one), but include a random suffix
	// anyway — the discount group sweeper incident (409
	// discount_group_name_conflict from concurrent CI jobs using a fixed
	// name) is exactly the failure mode a fixed destination could also hit
	// if Paddle enforces any uniqueness on it.
	suffix := randAccTestSuffix()
	destination := "https://example.com/webhook/" + suffix
	description := "Acc Test Notification " + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationSettingDeleted(resourceName),
		Steps: []resource.TestStep{
			{
				// Create, active left unset (defaults true) — the plain
				// create path, no follow-up update needed.
				Config: providerConfig + testAccNotificationSettingConfig(description, destination, `["transaction.billed"]`, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "description", description),
					resource.TestCheckResourceAttr(resourceName, "type", "url"),
					resource.TestCheckResourceAttr(resourceName, "destination", destination),
					resource.TestCheckResourceAttr(resourceName, "active", "true"),
					resource.TestCheckResourceAttr(resourceName, "subscribed_events.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "subscribed_events.0", "transaction.billed"),
					resource.TestCheckResourceAttr(resourceName, "traffic_source", "platform"),
					resource.TestCheckResourceAttr(resourceName, "include_sensitive_fields", "false"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
					resource.TestCheckResourceAttrSet(resourceName, "endpoint_secret_key"),
				),
			},
			{
				// Update: destination, subscribed_events, and active
				// (settable only via update, per the schema) all change in
				// place, no replacement (type is the only RequiresReplace
				// attribute here).
				Config: providerConfig + testAccNotificationSettingConfig(description, destination+"-updated", `["transaction.billed", "transaction.paid"]`, "false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "destination", destination+"-updated"),
					resource.TestCheckResourceAttr(resourceName, "subscribed_events.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "active", "false"),
				),
			},
			{
				// A second plan against the same config must be a no-op.
				Config:   providerConfig + testAccNotificationSettingConfig(description, destination+"-updated", `["transaction.billed", "transaction.paid"]`, "false"),
				PlanOnly: true,
			},
			{
				// Import.
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccPaddleNotificationSetting_activeFalseAtCreate exercises the
// Create-then-immediate-Update path in notification_setting_resource.go's
// Create() — Paddle's create endpoint doesn't accept `active` at all
// (always creates active: true), so setting `active = false` in the very
// first config exercises a follow-up update this resource issues right
// after creation. This is the one piece of Create() logic no other
// resource in this provider has, so it gets its own dedicated test rather
// than only being covered incidentally by the basic test's later Update
// step.
func TestAccPaddleNotificationSetting_activeFalseAtCreate(t *testing.T) {
	resourceName := "paddle_notification_setting.test"
	suffix := randAccTestSuffix()
	destination := "https://example.com/webhook/" + suffix
	description := "Acc Test Notification Inactive " + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationSettingDeleted(resourceName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccNotificationSettingConfig(description, destination, `["transaction.billed"]`, "false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "active", "false"),
				),
			},
		},
	})
}

func TestAccPaddleNotificationSettingDataSource_basic(t *testing.T) {
	dataSourceName := "data.paddle_notification_setting.test"
	suffix := randAccTestSuffix()
	destination := "https://example.com/webhook/" + suffix
	description := "Acc Test Notification For Lookup " + suffix

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNotificationSettingDeleted("paddle_notification_setting.test"),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccNotificationSettingConfig(description, destination, `["transaction.billed"]`, "") + `
data "paddle_notification_setting" "test" {
  id = paddle_notification_setting.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "id", "paddle_notification_setting.test", "id"),
					resource.TestCheckResourceAttr(dataSourceName, "description", description),
					resource.TestCheckResourceAttr(dataSourceName, "destination", destination),
					resource.TestCheckResourceAttrSet(dataSourceName, "endpoint_secret_key"),
				),
			},
		},
	})
}

func testAccNotificationSettingConfig(description, destination, subscribedEventsHCL, active string) string {
	activeLine := ""
	if active != "" {
		activeLine = "active = " + active
	}
	return fmt.Sprintf(`
resource "paddle_notification_setting" "test" {
  description       = %[1]q
  type              = "url"
  destination       = %[2]q
  subscribed_events = %[3]s
  %[4]s
}
`, description, destination, subscribedEventsHCL, activeLine)
}

// testAccCheckNotificationSettingDeleted is this resource's CheckDestroy.
// Unlike every other resource in this provider, "destroyed" here means
// genuinely gone (404 via client.IsNotFound), not archived — see
// client.DeleteNotificationSetting's comment on the real hard DELETE this
// resource's Delete() calls.
func testAccCheckNotificationSettingDeleted(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id := rs.Primary.ID

		_, err := newTestAccClient().GetNotificationSetting(context.Background(), id)
		if err == nil {
			return fmt.Errorf("notification setting %s still exists after destroy", id)
		}
		if !client.IsNotFound(err) {
			return fmt.Errorf("GetNotificationSetting(%s) after destroy: %w", id, err)
		}
		return nil
	}
}
