package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// testDeleteState builds a real tfsdk.State against a resource's own
// schema for a given model, mirroring the exact shape Terraform core would
// supply to Delete() — used to exercise Delete() directly as a unit test,
// since the bug under test (404-tolerance) lives in real request/response
// handling, not in a pure function these resources happen to expose.
func testDeleteState[M any](t *testing.T, r interface {
	Schema(context.Context, resource.SchemaRequest, *resource.SchemaResponse)
}, model M) resource.DeleteRequest {
	t.Helper()
	var schemaResp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("building test State: %v", diags)
	}
	return resource.DeleteRequest{State: state}
}

// TestProductDelete_TreatsAlreadyGone404AsSuccess and its Price/Discount
// siblings are regression tests for /code-review high findings: Delete()
// in all three resources hard-failed on any Archive error, including a 404
// for an object already removed outside Terraform (manually archived/
// deleted in the Paddle dashboard, or a prior partial destroy) — unlike
// Read(), which already tolerates exactly this via client.IsNotFound.
func TestProductDelete_TreatsAlreadyGone404AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"not_found","detail":"no such product"}}`))
	}))
	defer srv.Close()

	pr := &ProductResource{client: client.New(srv.URL, "test-key")}
	req := testDeleteState(t, pr, productResourceStateModel{
		ProductResourceModel: ProductResourceModel{
			ID:          types.StringValue("pro_gone"),
			Name:        types.StringValue("x"),
			TaxCategory: types.StringValue("standard"),
		},
		Timeouts: nullTimeouts(),
	})

	var resp resource.DeleteResponse
	pr.Delete(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("Delete() on an already-404 product produced errors, want success: %v", resp.Diagnostics)
	}
}

func TestPriceDelete_TreatsAlreadyGone404AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"not_found","detail":"no such price"}}`))
	}))
	defer srv.Close()

	pr := &PriceResource{client: client.New(srv.URL, "test-key")}
	m := baseModel()
	m.ID = types.StringValue("pri_gone")
	req := testDeleteState(t, pr, priceResourceStateModel{PriceResourceModel: m, Timeouts: nullTimeouts()})

	var resp resource.DeleteResponse
	pr.Delete(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("Delete() on an already-404 price produced errors, want success: %v", resp.Diagnostics)
	}
}

func TestDiscountDelete_TreatsAlreadyGone404AsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"type":"request_error","code":"not_found","detail":"no such discount"}}`))
	}))
	defer srv.Close()

	dr := &DiscountResource{client: client.New(srv.URL, "test-key")}
	m := baseDiscountModel()
	m.ID = types.StringValue("dsc_gone")
	req := testDeleteState(t, dr, discountResourceStateModel{DiscountResourceModel: m, Timeouts: nullTimeouts()})

	var resp resource.DeleteResponse
	dr.Delete(context.Background(), req, &resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("Delete() on an already-404 discount produced errors, want success: %v", resp.Diagnostics)
	}
}
