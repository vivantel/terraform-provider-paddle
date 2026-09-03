data "paddle_notification" "example" {
  # Look up directly by ID
  id = "ntf_..." # replace with a real notification ID

  # Or look up by notification_setting_id + status (exactly one must match):
  # notification_setting_id = "ntfset_..."
  # status                  = "delivered"
}
