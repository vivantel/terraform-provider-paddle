# See examples/lookup-then-act/main.tf for the full
# `resource "terraform_data" { lifecycle { action_trigger { ... } } }`
# wiring that actually invokes this action.

action "paddle_notification_replay" "example" {
  config {
    notification_id = "ntf_..." # replace with a real notification ID
  }
}
