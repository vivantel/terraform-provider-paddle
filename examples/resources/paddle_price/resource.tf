resource "paddle_product" "example" {
  name         = "Pro Plan"
  tax_category = "saas"
}

resource "paddle_price" "monthly" {
  product_id  = paddle_product.example.id
  description = "Pro Plan — Monthly" # internal only, never shown to customers
  name        = "Monthly"            # customer-facing

  unit_price = {
    amount        = "2900" # $29.00 for a 2-decimal currency — lowest denomination as a string
    currency_code = "USD"  # ISO 4217 code
  }

  billing_cycle = {
    interval  = "month" # day, week, month, or year
    frequency = 1
  }

  # tax_mode = "account_setting" # account_setting, external, internal, or location — defaults to account_setting
}
