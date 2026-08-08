resource "paddle_discount" "launch_promo" {
  type        = "percentage"
  amount      = "20" # 20% off
  description = "Launch week promotion"
  code        = "LAUNCH20"

  recur                       = true
  maximum_recurring_intervals = 3 # applies to the first 3 billing periods
}
