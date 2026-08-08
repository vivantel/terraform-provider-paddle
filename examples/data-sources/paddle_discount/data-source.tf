data "paddle_discount" "example" {
  id = "dsc_01h8xd4x1r5jz1y9xzq1dqk6mt"
}

output "discount_code" {
  value = data.paddle_discount.example.code
}
