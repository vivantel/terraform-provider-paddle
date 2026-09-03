# See examples/lookup-then-act/main.tf for the full
# `resource "terraform_data" { lifecycle { action_trigger { ... } } }`
# wiring that actually invokes this action.

action "paddle_subscription_charge" "example" {
  config {
    subscription_id = "sub_..."     # replace with a real subscription ID
    effective_from  = "immediately" # immediately or next_billing_period

    items = [
      {
        price_id = "pri_..." # replace with a real catalog price ID
        quantity = 1
      },
    ]

    # on_payment_failure = "prevent_change" # prevent_change or apply_change
    # receipt_data       = "Thank you for your purchase!" # max 1500 chars, only valid when effective_from is "immediately"
  }
}
