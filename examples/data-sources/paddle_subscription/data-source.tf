data "paddle_subscription" "example" {
  # Look up directly by ID
  id = "sub_..." # replace with a real subscription ID

  # Or look up by customer_id + status (exactly one must match):
  # customer_id = "ctm_..."
  # status      = "active"
}
