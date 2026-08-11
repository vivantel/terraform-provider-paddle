package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// These test the wire format directly — no HTTP involved — because every
// bug /code-review high found in the archive/clear-field path was a JSON
// marshaling mistake, not a networking one. A live sandbox call wouldn't
// have caught "field silently omitted instead of nulled" any faster than
// json.Marshal does here.

func TestProductJSON_NilOptionalFieldsMarshalAsExplicitNull(t *testing.T) {
	desc := "widget description"
	img := "https://example.com/widget.png"

	tests := []struct {
		name string
		p    Product
		want string
	}{
		{
			name: "unset optional fields marshal as null, not omitted",
			p:    Product{Name: "Widget", TaxCategory: "standard"},
			want: `{"name":"Widget","tax_category":"standard","description":null,"image_url":null}`,
		},
		{
			name: "set optional fields marshal their value",
			p:    Product{Name: "Widget", TaxCategory: "standard", Description: &desc, ImageURL: &img},
			want: `{"name":"Widget","tax_category":"standard","description":"widget description","image_url":"https://example.com/widget.png"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, b, tc.want)
		})
	}
}

func TestPriceJSON_NilNameMarshalsAsExplicitNull(t *testing.T) {
	name := "Widget Monthly"

	tests := []struct {
		name string
		p    Price
		want string
	}{
		{
			name: "unset name marshals as null, not omitted",
			p: Price{
				ProductID:   "pro_123",
				Description: "internal desc",
				UnitPrice:   Money{Amount: "1000", CurrencyCode: "USD"},
			},
			want: `{"product_id":"pro_123","description":"internal desc","unit_price":{"amount":"1000","currency_code":"USD"},"name":null}`,
		},
		{
			name: "set name marshals its value",
			p: Price{
				ProductID:   "pro_123",
				Description: "internal desc",
				UnitPrice:   Money{Amount: "1000", CurrencyCode: "USD"},
				Name:        &name,
			},
			want: `{"product_id":"pro_123","description":"internal desc","unit_price":{"amount":"1000","currency_code":"USD"},"name":"Widget Monthly"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.p)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, b, tc.want)
		})
	}
}

func TestDiscountJSON_NilOptionalFieldsMarshalAsExplicitNull(t *testing.T) {
	code := "SAVE10"

	tests := []struct {
		name string
		d    Discount
		want string
	}{
		{
			name: "unset nullable fields marshal as null, not omitted",
			d:    Discount{Description: "10% off", Type: "percentage", Amount: "10"},
			want: `{"description":"10% off","type":"percentage","amount":"10",
				"code":null,"currency_code":null,"maximum_recurring_intervals":null,
				"usage_limit":null,"restrict_to":null,"expires_at":null,"discount_group_id":null}`,
		},
		{
			name: "set code marshals its value, other nullable fields stay null",
			d:    Discount{Description: "10% off", Type: "percentage", Amount: "10", Code: &code},
			want: `{"description":"10% off","type":"percentage","amount":"10",
				"code":"SAVE10","currency_code":null,"maximum_recurring_intervals":null,
				"usage_limit":null,"restrict_to":null,"expires_at":null,"discount_group_id":null}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.d)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			assertJSONEqual(t, b, tc.want)
		})
	}
}

func TestDiscountJSON_ReadOnlyFieldsNeverSentInRequestBody(t *testing.T) {
	// A Discount built from a resource model (as toAPIDiscount in
	// discount_resource.go does) never sets TimesUsed/CreatedAt/UpdatedAt;
	// they're populated only by unmarshaling a response. Confirms they stay
	// omitted (not sent as 0/"") in that case.
	d := Discount{Description: "10% off", Type: "percentage", Amount: "10"}

	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, field := range []string{"times_used", "created_at", "updated_at"} {
		if strings.Contains(string(b), `"`+field+`"`) {
			t.Errorf("request body contains %q, want it entirely absent: %s", field, b)
		}
	}
}

func TestIsNotFound(t *testing.T) {
	// Regression coverage for /code-review high findings: the 404 check
	// (errors.As + StatusCode == 404) was copy-pasted identically in all
	// three resources' Read(), using a magic 404, and was missing entirely
	// from Delete() in all three — this is the shared helper both should
	// use instead.
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "404 APIError", err: &APIError{StatusCode: 404}, want: true},
		{name: "wrapped 404 APIError", err: fmt.Errorf("read: %w", &APIError{StatusCode: 404}), want: true},
		{name: "non-404 APIError", err: &APIError{StatusCode: 400}, want: false},
		{name: "non-APIError", err: errors.New("boom"), want: false},
		{name: "nil error", err: nil, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNotFound(tc.err); got != tc.want {
				t.Errorf("IsNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestStatusPatchJSON_OnlyStatusField(t *testing.T) {
	// Regression test for the archive bug: ArchiveProduct/ArchivePrice used
	// to build a zero-value Product/Price with just Status set, which
	// serialized required fields (name, tax_category, product_id,
	// description, unit_price) as empty strings since those lack
	// omitempty. statusPatch must be the only thing that PATCH body sends.
	b, err := json.Marshal(statusPatch{Status: "archived"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"status":"archived"}`)
}

// assertJSONEqual compares two JSON documents structurally (key order in
// the "want" literal is just for readability, not semantics).
func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotVal, wantVal any
	if err := json.Unmarshal(got, &gotVal); err != nil {
		t.Fatalf("got is not valid JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("want is not valid JSON: %v\n%s", err, want)
	}
	gotNorm, _ := json.Marshal(gotVal)
	wantNorm, _ := json.Marshal(wantVal)
	if string(gotNorm) != string(wantNorm) {
		t.Errorf("JSON mismatch:\n got:  %s\n want: %s", got, want)
	}
}

func TestDiscountGroupJSON_OnlyNameAndStatus(t *testing.T) {
	// Discount Groups' create/update body is genuinely just these two
	// fields (docs/decisions/0007) — this test exists to catch a future
	// accidental field addition to the wire struct that Paddle's API
	// doesn't accept, the same class of bug statusPatch was introduced to
	// prevent for archive bodies.
	g := DiscountGroup{Name: "VIP Customers"}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"name":"VIP Customers"}`)
}

func TestNotificationSettingCreateJSON_NoActiveField(t *testing.T) {
	// Active isn't accepted at create at all (confirmed against the real
	// API reference) — NotificationSettingCreate must not have an Active
	// field for a zero-value bool to accidentally send active: false.
	ns := NotificationSettingCreate{
		Description:      "Order events",
		Type:             "url",
		Destination:      "https://example.com/webhook",
		SubscribedEvents: []string{"transaction.billed"},
	}
	b, err := json.Marshal(ns)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), `"active"`) {
		t.Errorf("request body contains \"active\", want it entirely absent: %s", b)
	}
	assertJSONEqual(t, b, `{"description":"Order events","type":"url","destination":"https://example.com/webhook","subscribed_events":["transaction.billed"]}`)
}

func TestNotificationSettingUpdateJSON_NoTypeField(t *testing.T) {
	// Type is immutable after create (confirmed against the real API
	// reference) — NotificationSettingUpdate must not have a Type field.
	active := false
	ns := NotificationSettingUpdate{
		Description:      "Order events",
		Destination:      "https://example.com/webhook",
		Active:           &active,
		SubscribedEvents: []string{"transaction.billed"},
	}
	b, err := json.Marshal(ns)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), `"type"`) {
		t.Errorf("request body contains \"type\", want it entirely absent: %s", b)
	}
	assertJSONEqual(t, b, `{"description":"Order events","destination":"https://example.com/webhook","active":false,"subscribed_events":["transaction.billed"]}`)
}

func TestNotificationSettingJSON_SubscribedEventsUnmarshalsFromResponseObjectShape(t *testing.T) {
	// The response's subscribed_events is an array of event objects, not
	// strings (confirmed against the real API reference) — this is the
	// regression test for that asymmetry: if NotificationSetting ever
	// reused the request's []string shape, unmarshaling a real Paddle
	// response would fail outright, not silently misbehave.
	body := `{
		"id": "ntfset_1",
		"description": "Order events",
		"type": "url",
		"destination": "https://example.com/webhook",
		"active": true,
		"subscribed_events": [
			{"name": "transaction.billed", "description": "...", "group": "Transaction", "available_versions": [1]}
		],
		"endpoint_secret_key": "pdl_nsk_secret"
	}`
	var ns NotificationSetting
	if err := json.Unmarshal([]byte(body), &ns); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(ns.SubscribedEvents) != 1 || ns.SubscribedEvents[0].Name != "transaction.billed" {
		t.Errorf("SubscribedEvents = %+v, want one event named transaction.billed", ns.SubscribedEvents)
	}
	if ns.EndpointSecretKey != "pdl_nsk_secret" {
		t.Errorf("EndpointSecretKey = %q, want pdl_nsk_secret", ns.EndpointSecretKey)
	}
}

func TestCheckoutDomainJSON_UnmarshalsFullResponseShape(t *testing.T) {
	body := `{
		"id": "chedom_1",
		"domain": "checkout.example.com",
		"status": "approved",
		"payment_method_verification": {"apple_pay": {"status": "verified"}},
		"created_at": "2026-01-01T00:00:00Z",
		"updated_at": "2026-01-02T00:00:00Z"
	}`
	var d CheckoutDomain
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if d.Domain != "checkout.example.com" || d.Status != "approved" {
		t.Errorf("got = %+v, unexpected core fields", d)
	}
	if d.PaymentMethodVerification.ApplePay.Status != "verified" {
		t.Errorf("PaymentMethodVerification.ApplePay.Status = %q, want verified", d.PaymentMethodVerification.ApplePay.Status)
	}
}

func TestFriendlyErrorMessage_ParsesDetailAndCode(t *testing.T) {
	err := &APIError{StatusCode: 400, Body: `{"error":{"type":"request_error","code":"invalid_currency_code","detail":"currency_code must be a valid ISO 4217 code"},"meta":{"request_id":"abc"}}`}
	got := FriendlyErrorMessage(err)
	want := "currency_code must be a valid ISO 4217 code (invalid_currency_code)"
	if got != want {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, want)
	}
}

func TestFriendlyErrorMessage_DetailOnlyWhenCodeMissing(t *testing.T) {
	err := &APIError{StatusCode: 400, Body: `{"error":{"detail":"something went wrong"}}`}
	got := FriendlyErrorMessage(err)
	if got != "something went wrong" {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, "something went wrong")
	}
}

func TestFriendlyErrorMessage_FallsBackOnMalformedJSON(t *testing.T) {
	err := &APIError{StatusCode: 500, Body: "not json at all"}
	got := FriendlyErrorMessage(err)
	if got != err.Error() {
		t.Errorf("FriendlyErrorMessage = %q, want fallback to err.Error() = %q", got, err.Error())
	}
}

func TestFriendlyErrorMessage_FallsBackWhenDetailMissing(t *testing.T) {
	// Valid JSON, but not the documented envelope shape (empty detail) —
	// the type comment on APIError says the exact shape varies, so this
	// must fail safe to the raw body rather than surface an empty string.
	err := &APIError{StatusCode: 500, Body: `{"something":"else"}`}
	got := FriendlyErrorMessage(err)
	if got != err.Error() {
		t.Errorf("FriendlyErrorMessage = %q, want fallback to err.Error() = %q", got, err.Error())
	}
}

func TestFriendlyErrorMessage_NonAPIErrorReturnsErrorString(t *testing.T) {
	err := errors.New("boom")
	got := FriendlyErrorMessage(err)
	if got != "boom" {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, "boom")
	}
}

func TestFriendlyErrorMessage_UnwrapsWrappedAPIError(t *testing.T) {
	apiErr := &APIError{StatusCode: 400, Body: `{"error":{"detail":"bad request"}}`}
	wrapped := fmt.Errorf("do request: %w", apiErr)
	got := FriendlyErrorMessage(wrapped)
	if got != "bad request" {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, "bad request")
	}
}

func TestAdjustmentJSON_FullRequestShape(t *testing.T) {
	amount := "500"
	a := Adjustment{
		Action:        "refund",
		Type:          "partial",
		TaxMode:       "internal",
		TransactionID: "txn_01abc",
		Reason:        "customer requested refund",
		Items: []AdjustmentItem{
			{ItemID: "txnitm_01abc", Type: "partial", Amount: &amount},
		},
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// ID and Status are read-only / Paddle-assigned — must never appear in
	// a request body regardless of Go zero-value, same discipline every
	// other entity's Create body already follows (see e.g. Discount's
	// TimesUsed/CreatedAt/UpdatedAt).
	if strings.Contains(string(b), `"id"`) || strings.Contains(string(b), `"status"`) {
		t.Errorf("request body contains a read-only field, want id/status entirely absent: %s", b)
	}
	assertJSONEqual(t, b, `{
		"action": "refund",
		"type": "partial",
		"tax_mode": "internal",
		"transaction_id": "txn_01abc",
		"reason": "customer requested refund",
		"items": [{"item_id": "txnitm_01abc", "type": "partial", "amount": "500"}]
	}`)
}

func TestAdjustmentJSON_FullTypeOmitsItemsAndAmount(t *testing.T) {
	// A "full" adjustment needs no Items at all, and an Items entry of
	// type "full" needs no Amount (Paddle's API reference: Amount is
	// required only for partial-type items) — Amount's pointer+omitempty
	// must actually omit, not send amount:"".
	a := Adjustment{
		Action:        "credit",
		Type:          "full",
		TransactionID: "txn_01abc",
		Reason:        "goodwill credit",
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), `"items"`) {
		t.Errorf("request body contains \"items\" for a full adjustment with none given, want it absent: %s", b)
	}
	assertJSONEqual(t, b, `{"action":"credit","type":"full","transaction_id":"txn_01abc","reason":"goodwill credit"}`)
}

func TestAdjustmentJSON_ResponseStatusUnmarshals(t *testing.T) {
	body := `{
		"id": "adj_01abc",
		"action": "refund",
		"type": "partial",
		"transaction_id": "txn_01abc",
		"reason": "customer requested refund",
		"status": "approved"
	}`
	var a Adjustment
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if a.Status != "approved" {
		t.Errorf("Status = %q, want %q", a.Status, "approved")
	}
	if a.ID != "adj_01abc" {
		t.Errorf("ID = %q, want %q", a.ID, "adj_01abc")
	}
}

func TestSubscriptionCancelRequestJSON_OnlyEffectiveFrom(t *testing.T) {
	req := SubscriptionCancelRequest{EffectiveFrom: "immediately"}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"effective_from":"immediately"}`)
}

func TestSubscriptionPauseRequestJSON_OmitsUnsetResumeAtAndOnResume(t *testing.T) {
	req := SubscriptionPauseRequest{EffectiveFrom: "immediately"}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "resume_at") || strings.Contains(string(b), "on_resume") {
		t.Errorf("request body = %s, want resume_at/on_resume both absent when unset (indefinite pause, Paddle's own on_resume default)", b)
	}
	assertJSONEqual(t, b, `{"effective_from":"immediately"}`)
}

func TestSubscriptionPauseRequestJSON_ResumeAtRoundTrips(t *testing.T) {
	resumeAt := "2026-09-01T00:00:00Z"
	req := SubscriptionPauseRequest{EffectiveFrom: "immediately", ResumeAt: &resumeAt, OnResume: "continue_existing_billing_period"}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"effective_from":"immediately","resume_at":"2026-09-01T00:00:00Z","on_resume":"continue_existing_billing_period"}`)
}

func TestSubscriptionResumeRequestJSON(t *testing.T) {
	req := SubscriptionResumeRequest{EffectiveFrom: "immediately"}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"effective_from":"immediately"}`)
}

func TestSubscriptionChargeRequestJSON_OmitsUnsetReceiptData(t *testing.T) {
	req := SubscriptionChargeRequest{
		EffectiveFrom: "next_billing_period",
		Items:         []SubscriptionChargeItem{{PriceID: "pri_1", Quantity: 2}},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "receipt_data") {
		t.Errorf("request body = %s, want receipt_data absent when unset", b)
	}
	assertJSONEqual(t, b, `{"effective_from":"next_billing_period","items":[{"price_id":"pri_1","quantity":2}]}`)
}

func TestSubscriptionJSON_StatusUnmarshals(t *testing.T) {
	body := `{"id":"sub_01abc","status":"paused"}`
	var s Subscription
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if s.Status != "paused" {
		t.Errorf("Status = %q, want %q", s.Status, "paused")
	}
}

func TestTransactionCreateJSON_ManualBilledFixtureShape(t *testing.T) {
	req := TransactionCreate{
		Items:          []TransactionCreateItem{{PriceID: "pri_1", Quantity: 1}},
		CustomerID:     "ctm_1",
		AddressID:      "add_1",
		CollectionMode: "manual",
		Status:         "billed",
		BillingDetails: &BillingDetails{PaymentTerms: PaymentTerms{Interval: "day", Frequency: 14}},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{
		"items": [{"price_id":"pri_1","quantity":1}],
		"customer_id": "ctm_1",
		"address_id": "add_1",
		"collection_mode": "manual",
		"status": "billed",
		"billing_details": {"payment_terms": {"interval": "day", "frequency": 14}}
	}`)
}

func TestCustomerJSON_EmailOnly(t *testing.T) {
	b, err := json.Marshal(Customer{Email: "acc-test@example.com"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"email":"acc-test@example.com"}`)
}

func TestAddressJSON_CountryCodeOnly(t *testing.T) {
	b, err := json.Marshal(Address{CountryCode: "US"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"country_code":"US"}`)
}

func TestSubscriptionWithNextTransactionJSON_DecodesQueuedItems(t *testing.T) {
	body := `{
		"data": {
			"id": "sub_01abc",
			"status": "active",
			"next_transaction": {
				"items": [{"price_id": "pri_01abc", "quantity": 2}]
			}
		}
	}`
	var env subscriptionWithNextTransactionEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Data.ID != "sub_01abc" || env.Data.Status != "active" {
		t.Errorf("Data = %+v, want ID/Status populated from the embedded Subscription fields", env.Data)
	}
	if env.Data.NextTransaction == nil || len(env.Data.NextTransaction.Items) != 1 {
		t.Fatalf("NextTransaction = %+v, want one item", env.Data.NextTransaction)
	}
	item := env.Data.NextTransaction.Items[0]
	if item.PriceID != "pri_01abc" || item.Quantity != 2 {
		t.Errorf("item = %+v, want {pri_01abc 2}", item)
	}
}

func TestSubscriptionWithNextTransactionJSON_NilWhenAbsent(t *testing.T) {
	// A subscription with nothing queued for its next renewal beyond its
	// normal recurring items may omit next_transaction entirely.
	body := `{"data": {"id": "sub_01abc", "status": "active"}}`
	var env subscriptionWithNextTransactionEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if env.Data.NextTransaction != nil {
		t.Errorf("NextTransaction = %+v, want nil when the field is absent from the response", env.Data.NextTransaction)
	}
}

func TestFriendlyErrorMessage_IncludesFieldLevelErrors(t *testing.T) {
	apiErr := &APIError{
		StatusCode: 400,
		Body: `{"error":{"type":"request_error","code":"bad_request","detail":"Invalid request.",` +
			`"errors":[{"field":"transaction_id","message":"invalid input"}]}}`,
	}
	got := FriendlyErrorMessage(apiErr)
	want := "Invalid request. (bad_request); transaction_id: invalid input"
	if got != want {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, want)
	}
}

func TestFriendlyErrorMessage_MultipleFieldErrorsAllIncluded(t *testing.T) {
	apiErr := &APIError{
		StatusCode: 400,
		Body: `{"error":{"detail":"Invalid request.","errors":[` +
			`{"field":"a","message":"bad a"},{"field":"b","message":"bad b"}]}}`,
	}
	got := FriendlyErrorMessage(apiErr)
	want := "Invalid request.; a: bad a; b: bad b"
	if got != want {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, want)
	}
}

func TestFriendlyErrorMessage_NoFieldErrorsUnchanged(t *testing.T) {
	// Confirms the addition doesn't alter output for the common case
	// (no "errors" array at all) — every existing caller's expectations
	// stay intact.
	apiErr := &APIError{StatusCode: 400, Body: `{"error":{"detail":"bad request","code":"invalid_currency_code"}}`}
	got := FriendlyErrorMessage(apiErr)
	want := "bad request (invalid_currency_code)"
	if got != want {
		t.Errorf("FriendlyErrorMessage = %q, want %q", got, want)
	}
}

func TestTransactionCancelPatchJSON_OnlyStatusField(t *testing.T) {
	b, err := json.Marshal(transactionCancelPatch{Status: "canceled"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	assertJSONEqual(t, b, `{"status":"canceled"}`)
}

func TestTransactionJSON_ItemsPriceIsNestedNotFlat(t *testing.T) {
	// Confirms the real response shape, 2026-08-11: Transaction.Items'
	// price reference is nested under "price": {"id": ...}, not a flat
	// "price_id" field — this was assumed flat for most of this
	// session, silently breaking paddle_subscription_charge's item-set
	// matching the whole time (see TransactionItem's own comment).
	body := `{
		"id": "txn_01abc",
		"status": "completed",
		"origin": "subscription_charge",
		"items": [
			{"price": {"id": "pri_01abc"}, "quantity": 2}
		]
	}`
	var txn Transaction
	if err := json.Unmarshal([]byte(body), &txn); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(txn.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(txn.Items))
	}
	if txn.Items[0].Price.ID != "pri_01abc" {
		t.Errorf("Items[0].Price.ID = %q, want %q", txn.Items[0].Price.ID, "pri_01abc")
	}
	if txn.Items[0].Quantity != 2 {
		t.Errorf("Items[0].Quantity = %d, want 2", txn.Items[0].Quantity)
	}
}

func TestTransactionJSON_DetailsLineItemsHaveTheirOwnID(t *testing.T) {
	// Confirms the separate, second item shape create-adjustment's
	// item_id actually references — under details.line_items, a
	// different path from the top-level items field entirely (see
	// TransactionLineItem's own comment).
	body := `{
		"id": "txn_01abc",
		"status": "billed",
		"items": [{"price": {"id": "pri_01abc"}, "quantity": 1}],
		"details": {
			"line_items": [
				{"id": "txnitm_01abc", "price_id": "pri_01abc", "quantity": 1}
			]
		}
	}`
	var txn Transaction
	if err := json.Unmarshal([]byte(body), &txn); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if txn.Details == nil {
		t.Fatal("Details = nil, want populated")
	}
	if len(txn.Details.LineItems) != 1 {
		t.Fatalf("len(Details.LineItems) = %d, want 1", len(txn.Details.LineItems))
	}
	li := txn.Details.LineItems[0]
	if li.ID != "txnitm_01abc" || li.PriceID != "pri_01abc" || li.Quantity != 1 {
		t.Errorf("Details.LineItems[0] = %+v, want {ID:txnitm_01abc PriceID:pri_01abc Quantity:1}", li)
	}
}

func TestTransactionJSON_DetailsNilWhenAbsent(t *testing.T) {
	body := `{"id": "txn_01abc", "status": "billed"}`
	var txn Transaction
	if err := json.Unmarshal([]byte(body), &txn); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if txn.Details != nil {
		t.Errorf("Details = %+v, want nil when absent from the response", txn.Details)
	}
}
