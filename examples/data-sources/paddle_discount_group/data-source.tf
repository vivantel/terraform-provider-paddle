data "paddle_discount_group" "example" {
  id = "dsg_01h8xd4x1r5jz1y9xzq1dqk6mt"
}

output "discount_group_name" {
  value = data.paddle_discount_group.example.name
}
