data "paddle_subscriptions" "example" {
  # Filter by owning customer
  customer_id = "ctm_..." # replace with a real customer ID

  # Optionally narrow further by status:
  # status = "active"
}
