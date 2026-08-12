package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleNotificationsDataSource_basic — same leniency as
// notification_data_source_acc_test.go's singular equivalent: a
// notification can't be self-provisioned (Paddle's own record of an
// actual delivery attempt), so this lists whatever already exists in the
// sandbox account and skips cleanly if there are none.
func TestAccPaddleNotificationsDataSource_basic(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	notifications, err := c.ListNotificationsFiltered(context.Background(), client.NotificationListFilter{})
	if err != nil {
		t.Fatalf("ListNotificationsFiltered: %v", err)
	}
	if len(notifications) == 0 {
		t.Skip("no notifications exist in this sandbox account — nothing to list")
	}
	dataSourceName := "data.paddle_notifications.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
data "paddle_notifications" "test" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(dataSourceName, "notifications.#"),
					checkListAttrsSet(dataSourceName, "notifications", "id", "status", "notification_setting_id"),
				),
			},
		},
	})
}
