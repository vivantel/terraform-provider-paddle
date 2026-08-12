package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ resource.Resource = &NotificationSettingResource{}
var _ resource.ResourceWithImportState = &NotificationSettingResource{}

func NewNotificationSettingResource() resource.Resource {
	return &NotificationSettingResource{}
}

type NotificationSettingResource struct {
	client *client.Client
}

type NotificationSettingResourceModel struct {
	ID                     types.String `tfsdk:"id"`
	Description            types.String `tfsdk:"description"`
	Type                   types.String `tfsdk:"type"`
	Destination            types.String `tfsdk:"destination"`
	Active                 types.Bool   `tfsdk:"active"`
	SubscribedEvents       types.List   `tfsdk:"subscribed_events"`
	APIVersion             types.Int64  `tfsdk:"api_version"`
	IncludeSensitiveFields types.Bool   `tfsdk:"include_sensitive_fields"`
	TrafficSource          types.String `tfsdk:"traffic_source"`
	EndpointSecretKey      types.String `tfsdk:"endpoint_secret_key"`
}

// notificationSettingResourceStateModel is what Create/Read/Update/Delete
// decode Plan/State into — see productResourceStateModel's comment in
// product_resource.go for why this wrapper exists instead of a Timeouts
// field directly on NotificationSettingResourceModel
// (notification_setting_data_source.go decodes state into that exact type
// too, and its schema has no "timeouts" attribute).
type notificationSettingResourceStateModel struct {
	NotificationSettingResourceModel
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *NotificationSettingResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_setting"
}

func (r *NotificationSettingResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Paddle notification setting (a webhook or email destination) — see " +
			"https://developer.paddle.com/api-reference/notification-settings/overview. Unlike " +
			"`paddle_product`/`paddle_price`/`paddle_discount`/`paddle_discount_group`, Paddle has a real " +
			"hard delete for this entity; `terraform destroy` removes it entirely rather than archiving it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Paddle notification setting ID (`ntfset_...`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "1-500 characters.",
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`email` or `url`. Immutable after create — changing this replaces the notification setting.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators: []validator.String{
					stringvalidator.OneOf("email", "url"),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Webhook URL (for `type = \"url\"`) or email address (for `type = \"email\"`). 1-2048 characters.",
			},
			"active": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether Paddle should try to deliver events to this destination. Defaults to `true`. " +
					"Not settable at create per Paddle's API — if set to `false` here, this resource issues an immediate " +
					"follow-up update right after creation to apply it.",
				Default:       booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"subscribed_events": schema.ListAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Event type names to subscribe to (e.g. `transaction.billed`). Paddle's API is the source of truth for valid values — see https://developer.paddle.com/webhooks/overview for the full list; this schema doesn't replicate it.",
			},
			"api_version": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "API version used for event payloads sent to this destination. Omit for the account " +
					"default. Optional+Computed, not purely user-set: confirmed against the real sandbox that Paddle " +
					"returns its own default (e.g. 1) even when this is omitted rather than leaving it null, so modeling " +
					"this as Optional-only produced \"Provider produced inconsistent result after apply\" on the very " +
					"first real Create — the same class of fix as `paddle_discount`'s `code`.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"include_sensitive_fields": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether sensitive fields are included in event payloads. Defaults to `false`.",
				Default:             booldefault.StaticBool(false),
				PlanModifiers:       []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"traffic_source": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "`platform`, `simulation`, or `all`. Defaults to `platform`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
				Validators: []validator.String{
					stringvalidator.OneOf("platform", "simulation", "all"),
				},
			},
			"endpoint_secret_key": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Secret key Paddle uses to sign webhook payloads sent to this destination.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Read:   true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (r *NotificationSettingResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	c, diags := configureClient(req.ProviderData, "resource")
	resp.Diagnostics.Append(diags...)
	r.client = c
}

// subscribedEventNames extracts the plain event-name strings from the
// model's subscribed_events list — used to build every request body,
// since both NotificationSettingCreate and NotificationSettingUpdate want
// []string, not the response's []NotificationSettingEvent shape.
func subscribedEventNames(ctx context.Context, l types.List) ([]string, diag.Diagnostics) {
	var names []string
	diags := l.ElementsAs(ctx, &names, false)
	return names, diags
}

func toAPINotificationSettingCreate(ctx context.Context, m NotificationSettingResourceModel) (client.NotificationSettingCreate, diag.Diagnostics) {
	events, diags := subscribedEventNames(ctx, m.SubscribedEvents)
	if diags.HasError() {
		return client.NotificationSettingCreate{}, diags
	}
	ns := client.NotificationSettingCreate{
		Description:      m.Description.ValueString(),
		Type:             m.Type.ValueString(),
		Destination:      m.Destination.ValueString(),
		SubscribedEvents: events,
	}
	// IsUnknown() check matters here specifically because api_version
	// became Optional+Computed (see the schema comment on "api_version") —
	// on Create with it omitted from config, it's Unknown (not Null) until
	// Paddle fills in the account default. IsNull() alone is false for an
	// Unknown value too, so skipping the IsUnknown() check would send
	// api_version: 0 (ValueInt64() on an Unknown value silently returns the
	// zero value) instead of omitting the field.
	if !m.APIVersion.IsNull() && !m.APIVersion.IsUnknown() {
		v := int(m.APIVersion.ValueInt64())
		ns.APIVersion = &v
	}
	if !m.IncludeSensitiveFields.IsNull() && !m.IncludeSensitiveFields.IsUnknown() {
		v := m.IncludeSensitiveFields.ValueBool()
		ns.IncludeSensitiveFields = &v
	}
	if !m.TrafficSource.IsNull() && !m.TrafficSource.IsUnknown() {
		ns.TrafficSource = m.TrafficSource.ValueString()
	}
	return ns, diags
}

// toAPINotificationSettingUpdate builds the PATCH body. Unlike Create, it
// always carries Active (the only place it's settable — see the "active"
// schema attribute's MarkdownDescription) and never carries Type (rejected
// outright by Paddle's update endpoint, confirmed against the real API
// reference — see client.NotificationSettingUpdate's comment).
func toAPINotificationSettingUpdate(ctx context.Context, m NotificationSettingResourceModel) (client.NotificationSettingUpdate, diag.Diagnostics) {
	events, diags := subscribedEventNames(ctx, m.SubscribedEvents)
	if diags.HasError() {
		return client.NotificationSettingUpdate{}, diags
	}
	ns := client.NotificationSettingUpdate{
		Description:      m.Description.ValueString(),
		Destination:      m.Destination.ValueString(),
		SubscribedEvents: events,
	}
	if !m.Active.IsNull() && !m.Active.IsUnknown() {
		v := m.Active.ValueBool()
		ns.Active = &v
	}
	// IsUnknown() check matters here specifically because api_version
	// became Optional+Computed (see the schema comment on "api_version") —
	// on Create with it omitted from config, it's Unknown (not Null) until
	// Paddle fills in the account default. IsNull() alone is false for an
	// Unknown value too, so skipping the IsUnknown() check would send
	// api_version: 0 (ValueInt64() on an Unknown value silently returns the
	// zero value) instead of omitting the field.
	if !m.APIVersion.IsNull() && !m.APIVersion.IsUnknown() {
		v := int(m.APIVersion.ValueInt64())
		ns.APIVersion = &v
	}
	if !m.IncludeSensitiveFields.IsNull() && !m.IncludeSensitiveFields.IsUnknown() {
		v := m.IncludeSensitiveFields.ValueBool()
		ns.IncludeSensitiveFields = &v
	}
	if !m.TrafficSource.IsNull() && !m.TrafficSource.IsUnknown() {
		ns.TrafficSource = m.TrafficSource.ValueString()
	}
	return ns, diags
}

func fromAPINotificationSetting(ctx context.Context, ns client.NotificationSetting, m *NotificationSettingResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.StringValue(ns.ID)
	m.Description = types.StringValue(ns.Description)
	m.Type = types.StringValue(ns.Type)
	m.Destination = types.StringValue(ns.Destination)
	m.Active = types.BoolValue(ns.Active)
	m.APIVersion = types.Int64Value(int64(ns.APIVersion))
	m.IncludeSensitiveFields = types.BoolValue(ns.IncludeSensitiveFields)
	m.TrafficSource = types.StringValue(ns.TrafficSource)
	m.EndpointSecretKey = types.StringValue(ns.EndpointSecretKey)

	names := make([]string, len(ns.SubscribedEvents))
	for i, e := range ns.SubscribedEvents {
		names[i] = e.Name
	}
	listVal, elemDiags := types.ListValueFrom(ctx, types.StringType, names)
	diags.Append(elemDiags...)
	m.SubscribedEvents = listVal

	return diags
}

func (r *NotificationSettingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationSettingResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	createBody, diags := toAPINotificationSettingCreate(ctx, plan.NotificationSettingResourceModel)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cancel context.CancelFunc
	ctx, cancel, diags = resolveTimeout(ctx, plan.Timeouts, timeoutOpCreate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	created, err := r.client.CreateNotificationSetting(ctx, createBody)
	if err != nil {
		resp.Diagnostics.AddError("Error creating Paddle notification setting", client.FriendlyErrorMessage(err))
		return
	}

	// active isn't accepted by the create endpoint at all (see
	// client.NotificationSettingCreate) — Paddle always creates with
	// active: true. If the plan wants false, the only way to express that
	// via this API is an immediate follow-up update.
	if !plan.Active.IsNull() && !plan.Active.IsUnknown() && plan.Active.ValueBool() != created.Active {
		v := plan.Active.ValueBool()
		updated, err := r.client.UpdateNotificationSetting(ctx, created.ID, client.NotificationSettingUpdate{
			Description:      created.Description,
			Destination:      created.Destination,
			Active:           &v,
			SubscribedEvents: eventNamesOf(created),
		})
		if err != nil {
			resp.Diagnostics.AddError("Error setting initial active state on Paddle notification setting", client.FriendlyErrorMessage(err))
			return
		}
		created = updated
	}

	resp.Diagnostics.Append(fromAPINotificationSetting(ctx, *created, &plan.NotificationSettingResourceModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// eventNamesOf extracts subscribed event names from an already-fetched
// NotificationSetting — used only by Create's active-follow-up-update
// path above, where the update body must repeat subscribed_events (the
// update endpoint isn't a partial patch for this field) using the names
// Paddle just echoed back, not the plan's, to avoid re-deriving them from
// a types.List a second time for what's otherwise the same value.
func eventNamesOf(ns *client.NotificationSetting) []string {
	names := make([]string, len(ns.SubscribedEvents))
	for i, e := range ns.SubscribedEvents {
		names[i] = e.Name
	}
	return names
}

func (r *NotificationSettingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Fetch just id, not the whole model — same reasoning as every other
	// resource's Read() in this provider.
	var state notificationSettingResourceStateModel
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("id"), &state.ID)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("timeouts"), &state.Timeouts)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, state.Timeouts, timeoutOpRead, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	ns, err := r.client.GetNotificationSetting(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading Paddle notification setting", client.FriendlyErrorMessage(err))
		return
	}

	resp.Diagnostics.Append(fromAPINotificationSetting(ctx, *ns, &state.NotificationSettingResourceModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *NotificationSettingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan notificationSettingResourceStateModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state notificationSettingResourceStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateBody, diags := toAPINotificationSettingUpdate(ctx, plan.NotificationSettingResourceModel)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var cancel context.CancelFunc
	ctx, cancel, diags = resolveTimeout(ctx, plan.Timeouts, timeoutOpUpdate, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	updated, err := r.client.UpdateNotificationSetting(ctx, state.ID.ValueString(), updateBody)
	if err != nil {
		resp.Diagnostics.AddError("Error updating Paddle notification setting", client.FriendlyErrorMessage(err))
		return
	}

	resp.Diagnostics.Append(fromAPINotificationSetting(ctx, *updated, &plan.NotificationSettingResourceModel)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *NotificationSettingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationSettingResourceStateModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctx, cancel, diags := resolveTimeout(ctx, state.Timeouts, timeoutOpDelete, defaultOpTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	defer cancel()

	// A real hard DELETE, not archive-via-update (see
	// client.DeleteNotificationSetting's comment). A 404 means it's already
	// gone — successful destroy, not an error, same tolerance every other
	// resource's Delete() has for its own removal path.
	if err := r.client.DeleteNotificationSetting(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting Paddle notification setting", client.FriendlyErrorMessage(err))
	}
}

func (r *NotificationSettingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
