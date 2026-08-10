# A full-stack example wiring every resource this provider manages
# together, the way a real setup would use them — not isolated
# single-resource snippets. See README.md's Usage section for the
# minimal quick-start; this is the "how do these actually fit together"
# reference.

terraform {
  # >= 1.14.0 is required by this provider's actions — see
  # docs/decisions/0010-v3-scope-lifecycle-actions.md. This example
  # doesn't itself use one, but tracks the same floor as
  # examples/provider/provider.tf for consistency.
  required_version = ">= 1.14.0"

  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.3"
    }
  }
}

provider "paddle" {
  # api_key can also come from PADDLE_API_KEY
  # environment can also come from PADDLE_ENVIRONMENT — defaults to "sandbox"
  environment = "sandbox"
}

# A product with its recurring price.
resource "paddle_product" "pro" {
  name         = "Pro"
  tax_category = "saas"
  custom_data = jsonencode({
    internal_sku = "PRO-001"
  })
}

resource "paddle_price" "pro_monthly" {
  product_id  = paddle_product.pro.id
  description = "Pro tier, monthly"
  unit_price = {
    amount        = "2900"
    currency_code = "USD"
  }
  billing_cycle = {
    interval  = "month"
    frequency = 1
  }
}

# A discount group caps combined usage across every discount that
# belongs to it — created once, referenced by any number of discounts.
resource "paddle_discount_group" "promotions" {
  name = "2026 Promotions"
}

resource "paddle_discount" "launch_promo" {
  type        = "percentage"
  amount      = "20" # 20% off
  description = "Launch week promotion"
  code        = "LAUNCH20"

  discount_group_id = paddle_discount_group.promotions.id

  recur                       = true
  maximum_recurring_intervals = 3 # applies to the first 3 billing periods
}

# A webhook that fires when this product's price actually gets billed —
# the piece that turns "a catalog exists" into "my system finds out when
# something happens to it."
resource "paddle_notification_setting" "billing_events" {
  description = "Billing events for the Pro plan"
  type        = "url"
  destination = "https://example.com/webhooks/paddle"

  subscribed_events = [
    "transaction.billed",
    "transaction.paid",
    "subscription.created",
    "subscription.canceled",
  ]
}

# Look up something that already exists outside this config (e.g.
# approved via the Paddle dashboard, not created here) rather than
# managing it — paddle_checkout_domain has no matching resource at all,
# since Paddle's API has no create/update operation for it.
data "paddle_checkout_domain" "primary" {
  id = "chedom_..." # replace with a real checkout domain ID
}

output "monthly_price_id" {
  value = paddle_price.pro_monthly.id
}

output "checkout_domain_status" {
  value = data.paddle_checkout_domain.primary.status
}
