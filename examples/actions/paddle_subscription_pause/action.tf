# See examples/lookup-then-act/main.tf for the full
# `resource "terraform_data" { lifecycle { action_trigger { ... } } }`
# wiring that actually invokes this action.

action "paddle_subscription_pause" "example" {
  config {
    subscription_id = "sub_..."     # replace with a real subscription ID
    effective_from  = "immediately" # immediately or next_billing_period

    # resume_at = "2026-09-01T00:00:00Z" # RFC 3339 — omit for an indefinite pause
    # on_resume = "start_new_billing_period" # continue_existing_billing_period or start_new_billing_period
  }
}
