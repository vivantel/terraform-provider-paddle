package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
	"github.com/vivantel/terraform-provider-paddle/internal/provider/actions"
)

var _ provider.Provider = &PaddleProvider{}
var _ provider.ProviderWithActions = &PaddleProvider{}

type PaddleProvider struct {
	// version is set by main.go at build time (see .goreleaser.yml);
	// "dev" for local/unreleased builds.
	version string
}

type PaddleProviderModel struct {
	APIKey      types.String `tfsdk:"api_key"`
	Environment types.String `tfsdk:"environment"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PaddleProvider{version: version}
	}
}

func (p *PaddleProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "paddle"
	resp.Version = p.version
}

func (p *PaddleProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Paddle Billing catalog resources (products, prices, discounts, discount groups, notification settings); looks up checkout domains, subscriptions, transactions, customers, account events, and notification deliveries (data sources only); and provides actions for one-time lifecycle operations (adjustments, subscription cancel/pause/resume/charge) that don't have a resource lifecycle of their own. Unofficial — talks directly to Paddle's public REST API, no third party in the request path.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Paddle API key. Can also be set via the `PADDLE_API_KEY` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"environment": schema.StringAttribute{
				MarkdownDescription: "`sandbox` or `production`. Can also be set via the `PADDLE_ENVIRONMENT` environment variable. Defaults to `sandbox` if neither is set — deliberately, so a misconfigured provider block fails safe toward the environment that can't charge real cards.",
				Optional:            true,
			},
		},
	}
}

func (p *PaddleProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config PaddleProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := os.Getenv("PADDLE_API_KEY")
	if !config.APIKey.IsNull() && !config.APIKey.IsUnknown() {
		apiKey = config.APIKey.ValueString()
	}
	if apiKey == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("api_key"),
			"Missing Paddle API key",
			"Set the api_key provider attribute or the PADDLE_API_KEY environment variable.",
		)
		return
	}

	environment := os.Getenv("PADDLE_ENVIRONMENT")
	if !config.Environment.IsNull() && !config.Environment.IsUnknown() {
		environment = config.Environment.ValueString()
	}
	if environment == "" {
		environment = "sandbox"
	}

	var baseURL string
	switch environment {
	case "sandbox":
		baseURL = client.SandboxBaseURL
	case "production":
		baseURL = client.ProductionBaseURL
	default:
		resp.Diagnostics.AddAttributeError(
			path.Root("environment"),
			"Invalid environment",
			`environment must be "sandbox" or "production", got: `+environment,
		)
		return
	}

	// PADDLE_BASE_URL is a deliberately undocumented, internal-only
	// escape hatch — not a schema attribute, no public docs — that lets
	// this provider's own mock-server tests
	// (internal/provider/mockserver_test.go) point resource.Test's real
	// Create/Read/Update/Delete lifecycle at an httptest.Server instead
	// of Paddle's real sandbox/production URLs, via testAccProtoV6ProviderFactories.
	// Real users have no supported way to reach this: it's read directly
	// from the environment, never surfaced in the schema or
	// MarkdownDescription above, and unset in any real terraform.
	if override := os.Getenv("PADDLE_BASE_URL"); override != "" {
		baseURL = override
	}

	c := client.New(baseURL, apiKey)
	resp.DataSourceData = c
	resp.ResourceData = c
	resp.ActionData = c
}

func (p *PaddleProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewProductResource,
		NewPriceResource,
		NewDiscountResource,
		NewDiscountGroupResource,
		NewNotificationSettingResource,
	}
}

func (p *PaddleProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewProductDataSource,
		NewPriceDataSource,
		NewDiscountDataSource,
		NewDiscountGroupDataSource,
		NewNotificationSettingDataSource,
		NewCheckoutDomainDataSource,
		NewSubscriptionDataSource,
		NewTransactionDataSource,
		NewCustomerDataSource,
		NewEventsDataSource,
		NewNotificationDataSource,
		NewSubscriptionsDataSource,
		NewTransactionsDataSource,
		NewNotificationsDataSource,
		NewCustomersDataSource,
	}
}

// Actions — the first five (a refund/credit, and four subscription
// lifecycle ops) were this provider's first ever, see
// docs/decisions/0010-v3-scope-lifecycle-actions.md for why they're
// modeled as actions rather than resources: each is a one-time,
// irreversible "verb" against a Paddle entity with no real CRUD
// lifecycle of its own to reconcile on a later plan. The sixth
// (notification replay, v5) isn't irreversible/financial the same way —
// see docs/decisions/0012-v5-scope-pii-data-sources-timeouts-testing.md
// item 4 — but still fits "one endpoint, one verb, no CRUD lifecycle."
func (p *PaddleProvider) Actions(_ context.Context) []func() action.Action {
	return []func() action.Action{
		actions.NewAdjustmentAction,
		actions.NewSubscriptionCancelAction,
		actions.NewSubscriptionPauseAction,
		actions.NewSubscriptionResumeAction,
		actions.NewSubscriptionChargeAction,
		actions.NewNotificationReplayAction,
	}
}
