terraform {
  # >= 1.14.0 is required by this provider's actions (paddle_adjustment,
  # paddle_subscription_cancel/pause/resume/charge) — see
  # docs/decisions/0010-v3-scope-lifecycle-actions.md. Declared explicitly
  # rather than left implicit, since Terraform doesn't enforce an
  # action-using provider's version floor on its own.
  required_version = ">= 1.14.0"

  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.4"
    }
  }
}

# api_key/environment can be set here, or left unset to fall back to the
# PADDLE_API_KEY/PADDLE_ENVIRONMENT environment variables. environment
# defaults to "sandbox" if neither is set anywhere, so a misconfigured
# provider block fails safe toward the environment that can't charge real
# cards.
provider "paddle" {
  environment = "sandbox"
}
