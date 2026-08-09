resource "paddle_discount_group" "vip" {
  name = "VIP Customers"
}

resource "paddle_discount" "vip_20_off" {
  type              = "percentage"
  amount            = "20" # 20% off
  description       = "VIP group discount"
  discount_group_id = paddle_discount_group.vip.id
}
