resource "paddle_product" "example" {
  name         = "Pro Plan"
  tax_category = "saas"
}

resource "paddle_price" "monthly" {
  product_id  = paddle_product.example.id
  description = "Pro Plan — Monthly" # internal only, never shown to customers
  name        = "Monthly"            # customer-facing

  unit_price = {
    amount        = "2900" # $29.00 for a 2-decimal currency
    currency_code = "USD"
  }

  billing_cycle = {
    interval  = "month"
    frequency = 1
  }
}
