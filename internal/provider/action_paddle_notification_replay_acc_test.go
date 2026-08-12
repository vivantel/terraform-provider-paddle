package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestAccPaddleNotificationReplay_createsNewNotification reuses whatever
// notification already exists in the sandbox account with status
// delivered/failed (the only statuses Paddle's replay endpoint accepts,
// per docs/facts/0007-replay-endpoint-and-timeouts-module-confirmed.md) —
// the same lenient, self-provisioning-impossible pattern
// notification_data_source_acc_test.go's TestAccPaddleNotificationDataSource_basic
// already uses, since a notification can't be created via a direct API
// call at all. Confirms the replay genuinely created a *new* notification
// entity (a different ID, for the same notification_setting_id), not
// just that the action call didn't error — the plan's own Definition of
// Done for this step.
func TestAccPaddleNotificationReplay_createsNewNotification(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()

	replayable := findReplayableNotification(t, c)
	if replayable == nil {
		t.Skip("no delivered/failed notification exists in this sandbox account — nothing replayable")
	}

	before, err := c.ListNotificationsFiltered(ctx, client.NotificationListFilter{NotificationSettingID: replayable.NotificationSettingID})
	if err != nil {
		t.Fatalf("ListNotificationsFiltered (before): %v", err)
	}
	beforeIDs := make(map[string]bool, len(before))
	for _, n := range before {
		beforeIDs[n.ID] = true
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   tfVersionChecksActions,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
action "paddle_notification_replay" "test" {
  config {
    notification_id = %[1]q
  }
}

resource "terraform_data" "trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_notification_replay.test]
    }
  }
}
`, replayable.ID),
				PostApplyFunc: func() {
					after, err := c.ListNotificationsFiltered(ctx, client.NotificationListFilter{NotificationSettingID: replayable.NotificationSettingID})
					if err != nil {
						t.Fatalf("ListNotificationsFiltered (after): %v", err)
					}
					for _, n := range after {
						if !beforeIDs[n.ID] {
							// Found a genuinely new notification entity —
							// confirms replay via GET /notifications, not
							// just "the action call didn't error."
							return
						}
					}
					t.Errorf("no new notification appeared for notification_setting_id %s after replaying %s — replay may not have actually created a new entity", replayable.NotificationSettingID, replayable.ID)
				},
			},
		},
	})
}

// findReplayableNotification returns the first notification in the
// sandbox account with status delivered or failed — the only two
// statuses Paddle's replay endpoint accepts — or nil if none exist.
func findReplayableNotification(t *testing.T, c *client.Client) *client.Notification {
	t.Helper()
	for _, status := range []string{"delivered", "failed"} {
		notifications, err := c.ListNotificationsFiltered(context.Background(), client.NotificationListFilter{Status: status, Limit: 1})
		if err != nil {
			t.Fatalf("ListNotificationsFiltered(status=%s): %v", status, err)
		}
		if len(notifications) > 0 {
			return &notifications[0]
		}
	}
	return nil
}
