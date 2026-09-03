# See examples/lookup-then-act/main.tf for the full
# `resource "terraform_data" { lifecycle { action_trigger { ... } } }`
# wiring that actually invokes this action.

action "paddle_adjustment" "example" {
  config {
    action         = "credit"  # credit, refund, chargeback, chargeback_reverse, chargeback_warning, chargeback_warning_reverse, credit_reverse
    type           = "full"    # full or partial
    transaction_id = "txn_..." # replace with a real transaction ID
    reason         = "Customer requested refund"

    # Required by Paddle's API when type is "partial"; omit for "full".
    # items = [
    #   {
    #     item_id = "txnitm_..." # replace with a real transaction line item ID
    #     type    = "full"        # full, partial, tax, or proration
    #   },
    # ]
  }
}
