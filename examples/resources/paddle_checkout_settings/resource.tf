# One per Paddle account and environment. The page at this URL must have Paddle.js
# set up to open the checkout from the ?_ptxn= transaction ID Paddle appends.
resource "paddle_checkout_settings" "default" {
  default_url = "https://example.com/buy"
}
