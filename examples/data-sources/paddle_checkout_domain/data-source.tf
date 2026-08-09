data "paddle_checkout_domain" "example" {
  id = "chedom_01h8xd4x1r5jz1y9xzq1dqk6mt"
}

output "checkout_domain_status" {
  value = data.paddle_checkout_domain.example.status
}
