data "paddle_transaction" "example" {
  # Look up directly by ID
  id = "txn_..." # replace with a real transaction ID

  # Or look up by subscription_id / customer_id / status (exactly one must match):
  # subscription_id = "sub_..."
  # customer_id     = "ctm_..."
  # status          = "paid"
}
