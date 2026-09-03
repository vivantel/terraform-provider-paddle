package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccPaddleNotificationSettingSecretEphemeral_basic creates a real
// paddle_notification_setting, then fetches its endpoint_secret_key
// ephemerally in the same apply and confirms it matches the value the
// resource itself also captured (via echoprovider — see
// notification_setting_secret_ephemeral_mock_test.go's comment for why
// ephemeral results need this rather than the usual
// resource.TestCheckResourceAttr). CheckDestroy reuses
// testAccCheckNotificationSettingDeleted since the underlying
// paddle_notification_setting still needs real cleanup — the ephemeral
// resource itself never writes anything for a sweeper to find.
func TestAccPaddleNotificationSettingSecretEphemeral_basic(t *testing.T) {
	resourceName := "paddle_notification_setting.test"
	suffix := randAccTestSuffix()
	destination := "https://example.com/webhook/" + suffix
	description := "Acc Test Ephemeral Secret " + suffix

	factories := make(map[string]func() (tfprotov6.ProviderServer, error), len(testAccProtoV6ProviderFactories)+1)
	for k, v := range testAccProtoV6ProviderFactories {
		factories[k] = v
	}
	factories["echo"] = echoprovider.NewProviderServer()

	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0), // ephemeral resource support
		},
		ProtoV6ProviderFactories: factories,
		CheckDestroy:             testAccCheckNotificationSettingDeleted(resourceName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + testAccNotificationSettingConfig(description, destination, `["transaction.billed"]`, "") + `
ephemeral "paddle_notification_setting_secret" "test" {
  notification_setting_id = paddle_notification_setting.test.id
}

provider "echo" {
  data = ephemeral.paddle_notification_setting_secret.test
}

resource "echo" "test" {}
`,
				Check: resource.TestCheckResourceAttrSet(resourceName, "endpoint_secret_key"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						resourceName, tfjsonpath.New("endpoint_secret_key"),
						"echo.test", tfjsonpath.New("data").AtMapKey("endpoint_secret_key"),
						compare.ValuesSame(),
					),
				},
			},
		},
	})
}
