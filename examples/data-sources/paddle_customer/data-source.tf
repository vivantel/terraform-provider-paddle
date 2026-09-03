data "paddle_customer" "example" {
  # Look up directly by ID
  id = "ctm_..." # replace with a real customer ID

  # Or look up by email (exact match, exactly one must match):
  # email = "existing@example.com"
}
