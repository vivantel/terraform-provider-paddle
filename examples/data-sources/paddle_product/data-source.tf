data "paddle_product" "example" {
  id = "pro_01h8xce4qsr1a4b5xc0e6q3wr3"
}

output "product_name" {
  value = data.paddle_product.example.name
}
