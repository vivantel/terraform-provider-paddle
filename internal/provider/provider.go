package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

var _ provider.Provider = &PaddleProvider{}

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
		Description: "Manages Paddle Billing catalog resources (products, prices, discounts, discount groups, notification settings). Unofficial — talks directly to Paddle's public REST API, no third party in the request path.",
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

	c := client.New(baseURL, apiKey)
	resp.DataSourceData = c
	resp.ResourceData = c
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
	}
}
