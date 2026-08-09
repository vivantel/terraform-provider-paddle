terraform {
  required_providers {
    paddle = {
      source  = "vivantel/paddle"
      version = "~> 0.3"
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
