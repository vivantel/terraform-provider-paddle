ephemeral "paddle_notification_setting_secret" "example" {
  notification_setting_id = "ntfset_..." # replace with a real paddle_notification_setting id
}

# ephemeral.paddle_notification_setting_secret.example.endpoint_secret_key
# is never written to Terraform state. Reference it from a write-only
# attribute on whatever consumes it (e.g. a secrets-manager resource with
# a `*_wo` argument), not from a regular Computed/Optional attribute —
# those still persist to state regardless of where the value came from.
