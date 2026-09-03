# See examples/lookup-then-act/main.tf for the full
# `resource "terraform_data" { lifecycle { action_trigger { ... } } }`
# wiring that actually invokes this action.

action "paddle_subscription_cancel" "example" {
  config {
    subscription_id = "sub_..."     # replace with a real subscription ID
    effective_from  = "immediately" # immediately or next_billing_period
  }
}
