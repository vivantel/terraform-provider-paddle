resource "paddle_notification_setting" "orders" {
  description = "Order processing webhook"
  type        = "url"
  destination = "https://example.com/webhooks/paddle"

  subscribed_events = [
    "transaction.billed",
    "transaction.paid",
    "subscription.created",
  ]
}
