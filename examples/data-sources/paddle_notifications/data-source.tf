data "paddle_notifications" "example" {
  # Filter by owning notification destination
  notification_setting_id = "ntfset_..." # replace with a real notification setting ID

  # Optionally narrow further by status:
  # status = "delivered"
}
