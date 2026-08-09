data "paddle_notification_setting" "example" {
  id = "ntfset_01h8xd4x1r5jz1y9xzq1dqk6mt"
}

output "webhook_secret" {
  value     = data.paddle_notification_setting.example.endpoint_secret_key
  sensitive = true
}
