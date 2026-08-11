package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleNotificationDataSource_basic can't self-provision a
// notification the way most fixtures in this repo do — a notification is
// Paddle's own record of an actual delivery attempt, produced only once
// something in the account triggers an event against a configured
// notification_setting destination, not creatable via a direct API call.
// Per docs/plans/paddle-provider-v4.md Step 5, this test is deliberately
// lenient: it lists whatever notifications already exist in the sandbox
// account (e.g. from prior product/price/discount acceptance test runs,
// if any notification_setting is configured to receive those events) and
// skips cleanly if there are none, rather than requiring one specific
// notification to exist.
func TestAccPaddleNotificationDataSource_basic(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	notifications, err := c.ListNotificationsFiltered(context.Background(), client.NotificationListFilter{})
	if err != nil {
		t.Fatalf("ListNotificationsFiltered: %v", err)
	}
	if len(notifications) == 0 {
		t.Skip("no notifications exist in this sandbox account — nothing to look up (a notification is only produced by a real event against a configured notification_setting destination)")
	}
	want := notifications[0]
	dataSourceName := "data.paddle_notification.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_notification" "test" {
  id = %[1]q
}
`, want.ID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", want.ID),
					resource.TestCheckResourceAttr(dataSourceName, "status", want.Status),
					resource.TestCheckResourceAttrSet(dataSourceName, "notification_setting_id"),
					resource.TestCheckResourceAttrSet(dataSourceName, "logs.#"),
				),
			},
		},
	})
}

// TestAccPaddleNotificationDataSource_byFilter exercises the
// notification_setting_id/status filter-lookup path — only the id-lookup
// path had sandbox coverage before this (found via code review, same gap
// as paddle_transaction's). Same lenient precondition as
// TestAccPaddleNotificationDataSource_basic: this can't provision its own
// notification, so it picks whatever existing notification_setting_id/
// status pair happens to match exactly one notification in the account
// and skips cleanly if no such unambiguous pair exists.
func TestAccPaddleNotificationDataSource_byFilter(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	notifications, err := c.ListNotificationsFiltered(context.Background(), client.NotificationListFilter{})
	if err != nil {
		t.Fatalf("ListNotificationsFiltered: %v", err)
	}

	type key struct{ settingID, status string }
	counts := map[key]int{}
	for _, n := range notifications {
		counts[key{n.NotificationSettingID, n.Status}]++
	}
	var want *client.Notification
	for i, n := range notifications {
		if counts[key{n.NotificationSettingID, n.Status}] == 1 {
			want = &notifications[i]
			break
		}
	}
	if want == nil {
		t.Skip("no notification_setting_id/status pair in this sandbox account matches exactly one notification — nothing unambiguous to filter-look-up")
	}
	dataSourceName := "data.paddle_notification.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
data "paddle_notification" "test" {
  notification_setting_id = %[1]q
  status                   = %[2]q
}
`, want.NotificationSettingID, want.Status),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataSourceName, "id", want.ID),
					resource.TestCheckResourceAttr(dataSourceName, "notification_setting_id", want.NotificationSettingID),
				),
			},
		},
	})
}
