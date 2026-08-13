# The actual payoff v0.5.0's lookup data sources were built for: find an
# opaque Paddle ID entirely from inside Terraform (no copy-pasting from
# the Paddle dashboard), then feed it straight into an action — the same
# pattern this provider's own acceptance tests prove works
# (internal/provider/action_paddle_subscription_acc_test.go,
# internal/provider/transaction_data_source_acc_test.go's
# TestAccPaddleTransactionDataSource_feedsAdjustment), here as a real,
# readable config rather than test code. See README.md's Actions section
# for the general pattern this specializes.

terraform {
  # >= 1.14.0 is required by this provider's actions — see
  # docs/decisions/0010-v3-scope-lifecycle-actions.md.
  required_version = ">= 1.14.0"

  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.6"
    }
  }
}

provider "paddle" {
  # api_key can also come from PADDLE_API_KEY
  # environment can also come from PADDLE_ENVIRONMENT — defaults to "sandbox"
  environment = "sandbox"
}

# --- Subscription lookup -> cancel -------------------------------------
#
# Find a customer's active subscription by customer_id (something a real
# system actually has on hand — a webhook payload, a database row —
# unlike the opaque sub_... ID paddle_subscription_cancel itself needs),
# then cancel it. No sub_... ID hardcoded anywhere in this file.

data "paddle_subscription" "customer_sub" {
  customer_id = "ctm_..." # replace with a real customer ID
  status      = "active"
}

action "paddle_subscription_cancel" "cancel_customer_sub" {
  config {
    subscription_id = data.paddle_subscription.customer_sub.id
    effective_from  = "immediately"
  }
}

resource "terraform_data" "cancel_trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_subscription_cancel.cancel_customer_sub]
    }
  }
}

# --- Transaction lookup -> adjustment ------------------------------------
#
# Find a transaction by id, then feed its first billed line item's
# item_id straight into paddle_adjustment — item_id lives three JSON
# shapes deep in Paddle's raw API (see internal/client/lineitem.go's doc
# comment); paddle_transaction surfaces it directly.
#
# action = "credit", not "refund" — confirmed against the real sandbox
# (2026-08-12): Paddle rejects an item-level `items` array outright
# ("items are not allowed when the adjustment type is full") once
# `action = "refund"` and `type = "full"` are combined, but accepts the
# identical shape for `action = "credit"`. A whole-transaction, no-items
# refund (README.md's own basic Actions example) works fine; this
# example specifically demonstrates line_items[0].item_id feeding into
# an item-level adjustment, so credit is the combination that actually
# lets it do that.

data "paddle_transaction" "refund_target" {
  id = "txn_..." # replace with a real transaction ID
}

action "paddle_adjustment" "refund_line_item" {
  config {
    action         = "credit"
    type           = "full"
    transaction_id = data.paddle_transaction.refund_target.id
    reason         = "Customer requested refund"
    items = [
      {
        item_id = data.paddle_transaction.refund_target.line_items[0].item_id
        type    = "full"
      }
    ]
  }
}

resource "terraform_data" "refund_trigger" {
  lifecycle {
    action_trigger {
      events  = [after_create]
      actions = [action.paddle_adjustment.refund_line_item]
    }
  }
}

output "canceled_subscription_id" {
  value = data.paddle_subscription.customer_sub.id
}

output "refunded_transaction_id" {
  value = data.paddle_transaction.refund_target.id
}
