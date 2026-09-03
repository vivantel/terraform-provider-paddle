package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestMockPaddleNotificationSettingSecretEphemeral_Open reuses
// notificationSettingMockStore (notification_setting_resource_mock_test.go)
// directly rather than a second mock store — same package, same store
// shape, this test only ever GETs it.
//
// An ephemeral resource's result is never in state, so the usual
// resource.TestCheckResourceAttr (which reads state) can't see it.
// echoprovider (terraform-plugin-testing's own purpose-built helper for
// this) transfers the ephemeral value into a real resource's state via a
// companion "echo" provider so a test can assert on it — see that
// package's doc.go. echo.test's data attribute is Dynamic-typed (it has
// to accept any ephemeral resource's arbitrary schema), which is why this
// test uses ConfigStateChecks/tfjsonpath instead of the flatmap-style
// TestCheckResourceAttr every other mock test in this package uses.
func TestMockPaddleNotificationSettingSecretEphemeral_Open(t *testing.T) {
	store := &notificationSettingMockStore{
		data: &client.NotificationSetting{
			ID:                "ntfset_mock_1",
			Description:       "mock notification setting",
			Destination:       "https://example.com/webhook",
			Active:            true,
			EndpointSecretKey: "mock-secret-key",
		},
	}
	factories := newMockPaddleServer(t, store)
	factories["echo"] = echoprovider.NewProviderServer()

	resource.Test(t, resource.TestCase{
		IsUnitTest: true,
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0), // ephemeral resource support
		},
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: mockProviderConfig + `
ephemeral "paddle_notification_setting_secret" "test" {
  notification_setting_id = "ntfset_mock_1"
}

provider "echo" {
  data = ephemeral.paddle_notification_setting_secret.test
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"echo.test",
						tfjsonpath.New("data").AtMapKey("endpoint_secret_key"),
						knownvalue.StringExact("mock-secret-key"),
					),
					statecheck.ExpectKnownValue(
						"echo.test",
						tfjsonpath.New("data").AtMapKey("notification_setting_id"),
						knownvalue.StringExact("ntfset_mock_1"),
					),
				},
			},
		},
	})
}
