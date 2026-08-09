package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func mustListValue(t *testing.T, values ...string) types.List {
	t.Helper()
	l, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("ListValueFrom: %v", diags)
	}
	return l
}

func TestToAPINotificationSettingCreate_NeverSendsActive(t *testing.T) {
	// Regression coverage: NotificationSettingCreate has no Active field at
	// all (see client.go's comment) — this confirms the builder function
	// can't somehow route around that by, say, adding it via a different
	// mechanism, since the returned struct type itself makes it impossible.
	m := NotificationSettingResourceModel{
		Description:      types.StringValue("Order events"),
		Type:             types.StringValue("url"),
		Destination:      types.StringValue("https://example.com/webhook"),
		SubscribedEvents: mustListValue(t, "transaction.billed"),
		Active:           types.BoolValue(false),
	}
	got, diags := toAPINotificationSettingCreate(context.Background(), m)
	if diags.HasError() {
		t.Fatalf("toAPINotificationSettingCreate: %v", diags)
	}
	if got.Description != "Order events" || got.Type != "url" || got.Destination != "https://example.com/webhook" {
		t.Errorf("got = %+v, unexpected core fields", got)
	}
	if len(got.SubscribedEvents) != 1 || got.SubscribedEvents[0] != "transaction.billed" {
		t.Errorf("SubscribedEvents = %v, want [transaction.billed]", got.SubscribedEvents)
	}
}

func TestToAPINotificationSettingUpdate_CarriesActiveButNeverType(t *testing.T) {
	m := NotificationSettingResourceModel{
		Description:      types.StringValue("Order events"),
		Type:             types.StringValue("url"), // present in the model, must not leak into the update body
		Destination:      types.StringValue("https://example.com/webhook"),
		SubscribedEvents: mustListValue(t, "transaction.billed"),
		Active:           types.BoolValue(false),
	}
	got, diags := toAPINotificationSettingUpdate(context.Background(), m)
	if diags.HasError() {
		t.Fatalf("toAPINotificationSettingUpdate: %v", diags)
	}
	if got.Active == nil || *got.Active != false {
		t.Errorf("Active = %v, want pointer to false", got.Active)
	}
	// client.NotificationSettingUpdate has no Type field at the Go type
	// level — nothing to assert on got.Type here, the compiler already
	// guarantees it can't have been set. This test exists to document that
	// guarantee alongside the Active assertion above, not to check it at
	// runtime.
}

func TestFromAPINotificationSetting_ExtractsEventNamesFromResponseObjects(t *testing.T) {
	var m NotificationSettingResourceModel
	ns := client.NotificationSetting{
		ID:          "ntfset_1",
		Description: "Order events",
		Type:        "url",
		Destination: "https://example.com/webhook",
		Active:      true,
		SubscribedEvents: []client.NotificationSettingEvent{
			{Name: "transaction.billed"},
			{Name: "transaction.paid"},
		},
		EndpointSecretKey: "pdl_nsk_secret",
	}
	diags := fromAPINotificationSetting(context.Background(), ns, &m)
	if diags.HasError() {
		t.Fatalf("fromAPINotificationSetting: %v", diags)
	}

	var gotNames []string
	if diags := m.SubscribedEvents.ElementsAs(context.Background(), &gotNames, false); diags.HasError() {
		t.Fatalf("ElementsAs: %v", diags)
	}
	if len(gotNames) != 2 || gotNames[0] != "transaction.billed" || gotNames[1] != "transaction.paid" {
		t.Errorf("SubscribedEvents = %v, want [transaction.billed transaction.paid]", gotNames)
	}
	if m.EndpointSecretKey.ValueString() != "pdl_nsk_secret" {
		t.Errorf("EndpointSecretKey = %q, want pdl_nsk_secret", m.EndpointSecretKey.ValueString())
	}
}

func TestEventNamesOf_ExtractsNamesInOrder(t *testing.T) {
	ns := &client.NotificationSetting{
		SubscribedEvents: []client.NotificationSettingEvent{
			{Name: "a.b"}, {Name: "c.d"},
		},
	}
	got := eventNamesOf(ns)
	if len(got) != 2 || got[0] != "a.b" || got[1] != "c.d" {
		t.Errorf("eventNamesOf = %v, want [a.b c.d]", got)
	}
}
