package provider

import (
	"testing"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

func TestConfigureClient(t *testing.T) {
	realClient := client.New(client.SandboxBaseURL, "key")

	tests := []struct {
		name         string
		providerData any
		wantClient   *client.Client
		wantErr      bool
	}{
		{name: "nil ProviderData (provider not yet configured)", providerData: nil, wantClient: nil, wantErr: false},
		{name: "valid *client.Client", providerData: realClient, wantClient: realClient, wantErr: false},
		{name: "wrong type", providerData: "not a client", wantClient: nil, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, diags := configureClient(tc.providerData, "resource")
			if diags.HasError() != tc.wantErr {
				t.Errorf("diags.HasError() = %v, want %v (%v)", diags.HasError(), tc.wantErr, diags)
			}
			if c != tc.wantClient {
				t.Errorf("client = %v, want %v", c, tc.wantClient)
			}
		})
	}
}
