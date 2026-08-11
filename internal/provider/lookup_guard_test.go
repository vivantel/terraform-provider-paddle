package provider

import "testing"

// These guard against the same class of bug across paddle_subscription,
// paddle_transaction, and paddle_notification: a config that leaves id
// AND every filter unset must be a hard error (mirroring
// paddle_customer's existing "Missing lookup key" check), not a silent
// list-everything-and-hope-there's-exactly-one. Found via code review
// after this diff first shipped without it.

func TestSubscriptionFilterEmpty(t *testing.T) {
	cases := []struct {
		name                   string
		id, customerID, status string
		want                   bool
	}{
		{"all empty", "", "", "", true},
		{"id set", "sub_1", "", "", false},
		{"customer_id set", "", "ctm_1", "", false},
		{"status set", "", "", "active", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := subscriptionFilterEmpty(c.id, c.customerID, c.status); got != c.want {
				t.Errorf("subscriptionFilterEmpty(%q, %q, %q) = %v, want %v", c.id, c.customerID, c.status, got, c.want)
			}
		})
	}
}

func TestTransactionFilterEmpty(t *testing.T) {
	cases := []struct {
		name                                   string
		id, subscriptionID, customerID, status string
		want                                   bool
	}{
		{"all empty", "", "", "", "", true},
		{"id set", "txn_1", "", "", "", false},
		{"subscription_id set", "", "sub_1", "", "", false},
		{"customer_id set", "", "", "ctm_1", "", false},
		{"status set", "", "", "", "billed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := transactionFilterEmpty(c.id, c.subscriptionID, c.customerID, c.status); got != c.want {
				t.Errorf("transactionFilterEmpty(...) = %v, want %v", got, c.want)
			}
		})
	}
}

func TestNotificationFilterEmpty(t *testing.T) {
	cases := []struct {
		name                              string
		id, notificationSettingID, status string
		want                              bool
	}{
		{"all empty", "", "", "", true},
		{"id set", "ntf_1", "", "", false},
		{"notification_setting_id set", "", "ntfset_1", "", false},
		{"status set", "", "", "delivered", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := notificationFilterEmpty(c.id, c.notificationSettingID, c.status); got != c.want {
				t.Errorf("notificationFilterEmpty(...) = %v, want %v", got, c.want)
			}
		})
	}
}
