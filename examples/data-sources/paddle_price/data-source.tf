data "paddle_price" "example" {
  id = "pri_01h8xcqx4wtjb5vk8bxgcx3wz1"
}

output "price_amount" {
  value = data.paddle_price.example.unit_price.amount
}
