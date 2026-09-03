list "paddle_product" "all" {
  provider = paddle

  # Paddle's list-products endpoint takes no filters, so there's nothing to
  # set here — every product in the account is returned.
  config {}

  # Populates each result's full resource data (name, tax_category, ...),
  # not just its identity — needed for `terraform plan -generate-config-out`
  # to scaffold real resource blocks, not just import blocks.
  include_resource = true
}
