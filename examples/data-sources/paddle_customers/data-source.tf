data "paddle_customers" "example" {
  # Filter by email (exact match)
  email = "example.com" # replace with a real email domain or full address

  # Optionally narrow further by status:
  # status = "active"
}
