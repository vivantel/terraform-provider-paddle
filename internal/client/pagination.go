package client

// reachedLimit reports whether List*Filtered's pagination loop should
// stop early — limit <= 0 means unlimited (the original, still-default
// behavior every pre-existing caller of ListSubscriptionsFiltered/
// ListTransactionsFiltered/ListNotificationsFiltered relies on).
func reachedLimit(count, limit int) bool {
	return limit > 0 && count >= limit
}
