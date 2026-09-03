resource "paddle_discount_group" "vip" {
  name = "VIP Customers"
}

resource "paddle_discount" "vip_20_off" {
  type              = "percentage"
  amount            = "20" # 20% off
  description       = "VIP group discount"
  discount_group_id = paddle_discount_group.vip.id

  # code must be 1-32 alphanumeric chars, case-insensitive — omitted here so Paddle auto-generates one
  # enabled_for_checkout = true # defaults to true
  # mode = "standard" # standard or custom — defaults to standard
  # currency_code = "USD" # required when type is flat or flat_per_seat; not accepted for percentage
  # recur = false # whether the discount applies to every billing period — defaults to false
  # maximum_recurring_intervals = 3 # requires recur = true; omit for no limit
  # expires_at = "2026-12-31T23:59:59Z" # RFC 3339 — omit for a discount that never expires
  # restrict_to = ["pri_..."] # product or price IDs to restrict to — omit to apply to the whole catalog
}
