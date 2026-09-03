resource "paddle_notification_setting" "orders" {
  description = "Order processing webhook"
  type        = "url"                                 # email or url
  destination = "https://example.com/webhooks/paddle" # webhook URL or email address

  subscribed_events = [
    "transaction.billed",
    "transaction.paid",
    "subscription.created",
  ]

  # active = true # whether Paddle should try to deliver — defaults to true
  # api_version = 1 # API version for event payloads — omit for account default
  # include_sensitive_fields = false # defaults to false
  # traffic_source = "platform" # platform, simulation, or all — defaults to platform
}
