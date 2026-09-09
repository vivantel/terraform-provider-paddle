list "paddle_price" "all" {
  provider = paddle

  # Paddle's list-prices endpoint takes no filters, so there's nothing to
  # set here — every price in the account is returned.
  config {}

  # Populates each result's full resource data (product_id, unit_price,
  # billing_cycle, ...), not just its identity — needed for `terraform plan
  # -generate-config-out` to scaffold real resource blocks, not just import
  # blocks.
  include_resource = true
}
