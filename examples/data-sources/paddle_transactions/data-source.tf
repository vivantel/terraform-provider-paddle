data "paddle_transactions" "example" {
  # Filter by owning subscription
  subscription_id = "sub_..." # replace with a real subscription ID

  # Optionally narrow further:
  # customer_id = "ctm_..."
  # status      = "paid"
}
