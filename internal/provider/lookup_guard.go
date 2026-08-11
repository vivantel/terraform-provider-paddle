package provider

// subscriptionFilterEmpty, transactionFilterEmpty, notificationFilterEmpty
// report whether a lookup data source's config leaves id AND every filter
// unset — the same "Missing lookup key" case paddle_customer already
// guards against explicitly. Without this guard, an all-empty config
// falls through to an unfiltered list-everything-in-the-account call,
// and if the account happens to have exactly one matching record (common
// in a sandbox or early-stage prod account), that gets silently returned
// as "the match" with no indication no filter was actually applied — a
// real risk for paddle_subscription/paddle_transaction, whose output can
// feed directly into a real cancel/pause/resume/charge/adjustment
// action. Found via code review, docs/plans/paddle-provider-v4.md.

func subscriptionFilterEmpty(id, customerID, status string) bool {
	return id == "" && customerID == "" && status == ""
}

func transactionFilterEmpty(id, subscriptionID, customerID, status string) bool {
	return id == "" && subscriptionID == "" && customerID == "" && status == ""
}

func notificationFilterEmpty(id, notificationSettingID, status string) bool {
	return id == "" && notificationSettingID == "" && status == ""
}
