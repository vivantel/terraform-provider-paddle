data "paddle_events" "example" {
  # Filter by event type(s)
  type = [
    "product.created",
    "subscription.created",
  ]

  # Leave unset to list every event type (subject to Paddle's 90-day retention).
}
