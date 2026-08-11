package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/vivantel/terraform-provider-paddle/internal/client"
)

// TestMain wires up terraform-plugin-testing's -sweep flag handling.
// Required once per package for resource.AddTestSweepers to have any
// effect — see docs/decisions/0009-tflog-observability-and-acceptance-test-sweepers.md.
func TestMain(m *testing.M) {
	resource.TestMain(m)
}

// acceptance test configs across product/price/discount already name or
// describe their objects with "Acc Test" somewhere in the string (see
// *_acc_test.go) — sweepers reuse that same substring, case-insensitively,
// rather than introducing a second naming convention. No renaming of
// existing test configs was needed; they were already consistent enough
// for this to work.
const accTestMarker = "acc test"

func isAccTestName(s string) bool {
	return strings.Contains(strings.ToLower(s), accTestMarker)
}

// sweepClient builds a client.Client straight from the sandbox API key,
// mirroring newTestAccClient in provider_test.go — sweepers run outside any
// *testing.T context (resource.Sweeper.F takes just a region string), so
// they can't reuse that helper directly, but the construction is identical.
// Returns nil if PADDLE_API_KEY isn't set, so callers can skip cleanly
// rather than sweeping with an empty key and getting an auth error.
func sweepClient() *client.Client {
	key := os.Getenv("PADDLE_API_KEY")
	if key == "" {
		return nil
	}
	return client.New(client.SandboxBaseURL, key)
}

func init() {
	resource.AddTestSweepers("paddle_price", &resource.Sweeper{
		Name: "paddle_price",
		F:    sweepPrices,
	})
	resource.AddTestSweepers("paddle_discount", &resource.Sweeper{
		Name: "paddle_discount",
		F:    sweepDiscounts,
	})
	resource.AddTestSweepers("paddle_product", &resource.Sweeper{
		Name: "paddle_product",
		// Prices reference a product_id — sweep prices first so a leaked
		// price never outlives the product it points at, even though
		// archiving (not deleting) means this ordering isn't strictly
		// required for correctness today. Cheap to get right regardless.
		Dependencies: []string{"paddle_price"},
		F:            sweepProducts,
	})
	resource.AddTestSweepers("paddle_discount_group", &resource.Sweeper{
		Name: "paddle_discount_group",
		// Discounts may reference a discount_group_id — sweep discounts
		// first for the same reason paddle_product sweeps paddle_price
		// first above.
		Dependencies: []string{"paddle_discount"},
		F:            sweepDiscountGroups,
	})
	resource.AddTestSweepers("paddle_notification_setting", &resource.Sweeper{
		Name: "paddle_notification_setting",
		F:    sweepNotificationSettings,
	})
	resource.AddTestSweepers("paddle_test_fixture_customer", &resource.Sweeper{
		Name: "paddle_test_fixture_customer",
		F:    sweepTestFixtureCustomers,
	})
	resource.AddTestSweepers("paddle_test_subscription_charge", &resource.Sweeper{
		Name: "paddle_test_subscription_charge",
		F:    sweepTestSubscriptionCharges,
	})
}

// isAccTestCustomerEmail is the Customer-specific equivalent of
// isAccTestName — Customer has no name field at fixture-creation time
// (see client.CreateCustomer), and an email local-part can't contain the
// space isAccTestName's "acc test" substring relies on, so fixture
// customer emails use "acctest" instead (see
// action_paddle_adjustment_acc_test.go's fixture helper).
func isAccTestCustomerEmail(email string) bool {
	return strings.Contains(strings.ToLower(email), "acctest")
}

// cancelOrCreditTransaction cleans up one real transaction/invoice record
// left over from paddle_adjustment/paddle_subscription_charge's
// acceptance-test fixtures — found necessary the hard way, 2026-08-11:
// running those tests against the real sandbox generates real, permanent,
// notified invoices (client.CreateTransaction/ChargeSubscription's own
// comments have the full account). Still-outstanding transactions
// (draft/ready/billed) get canceled outright — a real, permanent removal
// of the obligation (see client.CancelTransaction's comment). Once
// completed (paid) or past_due, canceling is no longer possible; falls
// back to a full credit adjustment instead, which doesn't remove the
// record but zeroes out what's owed. Treats a 404 from either path (the
// transaction is already gone) as success, same tolerance every other
// sweeper in this file already has for its own entity.
// shouldAttemptCancel reports whether cancelOrCreditTransaction should
// even try CancelTransaction before falling back to credit/refund — false
// only for "completed", the one status a cancel attempt is *guaranteed*
// to be rejected for (a paid transaction can only be refunded/credited,
// never canceled outright). Found the hard way, 2026-08-11: sweeping a
// real backlog of leaked subscription-charge transactions (always
// "completed" — see the comment below), every single Cancel attempt
// against one still went through the client's full retry-with-backoff
// path before falling through to the working credit/refund path, roughly
// doubling this sweeper's real-world runtime under Paddle's rate
// limiting for no benefit — an outcome already known in advance from the
// status alone. Every other status still attempts Cancel first,
// unchanged: cancel-then-fall-back-to-credit remains the right shape for
// draft/ready/billed/past_due, where whether Cancel will succeed isn't
// knowable without asking Paddle.
func shouldAttemptCancel(status string) bool {
	return status != "completed"
}

func cancelOrCreditTransaction(ctx context.Context, c *client.Client, txn client.Transaction) error {
	if shouldAttemptCancel(txn.Status) {
		if err := c.CancelTransaction(ctx, txn.ID); err == nil || client.IsNotFound(err) {
			return nil
		}
	}
	// Falls through here either because Cancel was skipped (status ==
	// "completed", see shouldAttemptCancel) or because it was attempted
	// and failed — most likely because the transaction is past the
	// cancelable draft/ready/billed states. Paddle's adjustment action
	// must match the transaction's actual status: "refund" for a paid
	// (completed) transaction, "credit" for an unpaid one (billed/
	// past_due) — found the hard way, 2026-08-11: this sweeper originally
	// hardcoded "credit" unconditionally, and Paddle rejected it for
	// every one of these (real-world) subscription-charge transactions,
	// which are auto-collected and therefore "completed" (paid) by the
	// time this sweeper runs, not "billed" — "credit" doesn't apply to an
	// already-paid transaction.
	action := "credit"
	if txn.Status == "completed" {
		action = "refund"
	}
	// Items must be listed explicitly even for a full adjustment — found
	// the hard way, 2026-08-11: "type: full" alone (no items array) was
	// rejected twice in a row with "Items: must be greater than 0",
	// despite the API reference's prose implying items are only required
	// for a partial adjustment. The item_id each entry needs is on
	// Transaction.Details.LineItems, not the top-level Items field this
	// txn value already has (see client.LineItemIDs' comment) —
	// re-fetched per transaction via GetTransaction rather than assumed
	// present on whatever list call found this txn in the first place.
	full, err := c.GetTransaction(ctx, txn.ID)
	if err != nil {
		return fmt.Errorf("fetching full transaction detail for item IDs: %w", err)
	}
	ids := client.LineItemIDs(full)
	items := make([]client.AdjustmentItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, client.AdjustmentItem{ItemID: id, Type: "full"})
	}
	_, err = c.CreateAdjustment(ctx, client.Adjustment{
		Action:        action,
		Type:          "full",
		TransactionID: txn.ID,
		Reason:        "sweeper cleanup of a leaked test fixture transaction",
		Items:         items,
	})
	if err == nil || client.IsNotFound(err) {
		return nil
	}
	// A repeat sweep run matches every subscription_charge-origin
	// transaction unconditionally (see this sweeper's own comment on why
	// — no cheap way to mark one as "already handled"), so a transaction
	// a previous run already fully refunded/credited gets matched again
	// here every time. adjustment_transaction_item_has_already_been_fully_adjusted
	// means exactly that: already cleaned up, not a real failure — found
	// running a second sweep after the first had already handled these,
	// 2026-08-11. Treated as success so it doesn't look like a bug on
	// every future run.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && strings.Contains(apiErr.Body, "adjustment_transaction_item_has_already_been_fully_adjusted") {
		return nil
	}
	return err
}

func sweepTestFixtureCustomers(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_test_fixture_customer sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	customers, err := c.ListTestFixtureCustomers(ctx)
	if err != nil {
		return err
	}
	var matched, swept, txnsSwept, skippedTooRecent int
	now := time.Now()
	for _, cust := range customers {
		if cust.Status == "archived" || !isAccTestCustomerEmail(cust.Email) {
			continue
		}
		matched++
		// Contention guard: this sweeper and the acceptance suite both
		// run against the same live sandbox account with no
		// coordination between them (found via a real question about
		// this exact race, 2026-08-11) — a sweep that happens to run
		// while an acceptance test is still mid-flight could otherwise
		// archive/cancel a fixture that test hasn't finished using yet.
		// Now largely a secondary safety net rather than the primary
		// risk: every fixture-creation site using this sweeper's
		// naming convention also cleans up via its own t.Cleanup as of
		// this same fix (see createAdjustmentFixtureTransaction), so
		// this guard mainly protects a run that's still in flight
		// *when* a sweep happens to fire, not the common case anymore.
		if tooRecentToSweep(cust.CreatedAt, now, sweepMinAge) {
			skippedTooRecent++
			continue
		}
		txns, err := c.ListTransactionsByCustomer(ctx, cust.ID)
		if err != nil {
			log.Printf("[WARN] failed to list transactions for leaked test fixture customer %s (%s): %s", cust.ID, cust.Email, err)
		}
		for _, txn := range txns {
			if err := cancelOrCreditTransaction(ctx, c, txn); err != nil {
				log.Printf("[WARN] failed to cancel/credit leaked test fixture transaction %s (customer %s): %s", txn.ID, cust.ID, err)
				continue
			}
			txnsSwept++
		}
		if err := c.ArchiveTestFixtureCustomer(ctx, cust.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test fixture customer %s (%s): %s", cust.ID, cust.Email, err)
			continue
		}
		swept++
	}
	log.Printf("[INFO] paddle_test_fixture_customer sweeper: matched %d, swept %d, transactions canceled/credited %d, skipped as too recent %d", matched, swept, txnsSwept, skippedTooRecent)
	return nil
}

// sweepTestSubscriptionCharges cleans up real transactions
// paddle_subscription_charge's acceptance test creates against
// PADDLE_TEST_SUBSCRIPTION_ID (see findTestSubscription in
// action_paddle_subscription_acc_test.go) — that subscription is by
// definition a dedicated test fixture, so every subscription_charge
// -origin transaction against it is swept unconditionally, no naming
// filter needed (unlike every naming-convention-matched sweeper above).
// No-ops (not an error) if PADDLE_TEST_SUBSCRIPTION_ID isn't set — there's
// nothing to sweep without knowing which subscription is the test one.
func sweepTestSubscriptionCharges(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_test_subscription_charge sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	subID := os.Getenv("PADDLE_TEST_SUBSCRIPTION_ID")
	if subID == "" {
		log.Printf("[INFO] paddle_test_subscription_charge sweeper: PADDLE_TEST_SUBSCRIPTION_ID not set, nothing to sweep")
		return nil
	}
	ctx := context.Background()
	txns, err := c.ListSubscriptionChargeTransactions(ctx, subID)
	if err != nil {
		return err
	}
	var matched, swept int
	for _, txn := range txns {
		matched++
		if err := cancelOrCreditTransaction(ctx, c, txn); err != nil {
			// status included — diagnostic added 2026-08-11 after a real
			// sweep run showed every failure taking ~120s (a full
			// retry-with-backoff cycle each on CancelTransaction *and*
			// GetTransaction), which shouldAttemptCancel's "completed"
			// skip should have halved to ~60s if these transactions are
			// really all "completed" as assumed — this makes the actual
			// status visible in the log instead of guessing.
			log.Printf("[WARN] failed to cancel/credit leaked subscription-charge transaction %s (status %q, subscription %s): %s", txn.ID, txn.Status, subID, err)
			continue
		}
		swept++
	}
	log.Printf("[INFO] paddle_test_subscription_charge sweeper: matched %d, swept %d", matched, swept)
	return nil
}

func sweepProducts(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_product sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	return sweepMatchingProducts(context.Background(), c, func(p client.Product) bool {
		return p.Status != "archived" && isAccTestName(p.Name)
	})
}

// sweepMatchingProducts does the actual list-then-archive work behind
// sweepProducts, parameterized on which products to touch. sweepProducts
// itself always passes the broad isAccTestName match (that's the point of
// a sweeper — clean up anything leaked, regardless of which test run
// created it), but a verification test needs to scope this to only the one
// object *it* created: this package's acceptance tests run as `push` and
// `pull_request` CI jobs concurrently against the same shared sandbox
// account, so a verification test that invoked the broad match would race
// with, and potentially clean up, a completely unrelated concurrent job's
// still-in-progress fixture — confirmed the hard way when an equivalent
// notification-setting version of this test did exactly that (see
// TestAccSweepNotificationSettings_DeletesLeakedTestObjects's comment).
func sweepMatchingProducts(ctx context.Context, c *client.Client, match func(client.Product) bool) error {
	products, err := c.ListProducts(ctx)
	if err != nil {
		return err
	}
	var matched, swept int
	for _, p := range products {
		if !match(p) {
			continue
		}
		matched++
		if err := c.ArchiveProduct(ctx, p.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test product %s (%s): %s", p.ID, p.Name, err)
			continue
		}
		swept++
	}
	// A count on every run, not just on failure — a clean run with zero
	// [WARN] lines previously only proved nothing it tried to sweep
	// failed, not whether it found (and swept) anything at all.
	log.Printf("[INFO] paddle_product sweeper: matched %d, swept %d", matched, swept)
	return nil
}

func sweepPrices(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_price sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	prices, err := c.ListPrices(ctx)
	if err != nil {
		return err
	}
	var matched, swept int
	for _, p := range prices {
		if p.Status == "archived" || !isAccTestName(p.Description) {
			continue
		}
		matched++
		if err := c.ArchivePrice(ctx, p.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test price %s (%s): %s", p.ID, p.Description, err)
			continue
		}
		swept++
	}
	log.Printf("[INFO] paddle_price sweeper: matched %d, swept %d", matched, swept)
	return nil
}

// TestAccSweepProducts_ArchivesLeakedTestObjects exercises the real
// list-then-archive mechanics behind sweepProducts against the real
// sandbox: creates a product outside Terraform entirely — the exact
// "leaked between test runs" scenario sweepers exist for — confirms the
// sweep archives it. Scoped to only this fixture's ID (via
// sweepMatchingProducts, not sweepProducts itself) rather than the broad
// isAccTestName match the real sweeper uses — see
// sweepMatchingProducts' comment for why: this package's acceptance
// tests run as concurrent CI jobs against one shared sandbox account, and
// the broad match would risk touching another concurrent job's
// in-progress fixture, not just this test's own.
func TestAccSweepProducts_ArchivesLeakedTestObjects(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()

	leaked, err := c.CreateProduct(ctx, client.Product{
		Name:        "Acc Test Sweeper Leaked Product",
		TaxCategory: "standard",
	})
	if err != nil {
		t.Fatalf("CreateProduct (leaked fixture): %v", err)
	}
	t.Cleanup(func() {
		_ = c.ArchiveProduct(ctx, leaked.ID) // best-effort; the sweep itself is what's under test
	})

	if err := sweepMatchingProducts(ctx, c, func(p client.Product) bool { return p.ID == leaked.ID }); err != nil {
		t.Fatalf("sweepMatchingProducts: %v", err)
	}

	got, err := c.GetProduct(ctx, leaked.ID)
	if err != nil {
		t.Fatalf("GetProduct after sweep: %v", err)
	}
	if got.Status != "archived" {
		t.Errorf("status after sweep = %q, want archived — sweep did not clean up the leaked object", got.Status)
	}
}

func sweepDiscounts(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_discount sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	discounts, err := c.ListDiscounts(ctx)
	if err != nil {
		return err
	}
	var matched, swept int
	for _, d := range discounts {
		if d.Status == "archived" || !isAccTestName(d.Description) {
			continue
		}
		matched++
		if err := c.ArchiveDiscount(ctx, d.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test discount %s (%s): %s", d.ID, d.Description, err)
			continue
		}
		swept++
	}
	log.Printf("[INFO] paddle_discount sweeper: matched %d, swept %d", matched, swept)
	return nil
}

func sweepDiscountGroups(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_discount_group sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	ctx := context.Background()
	groups, err := c.ListDiscountGroups(ctx)
	if err != nil {
		return err
	}
	var matched, swept int
	for _, g := range groups {
		if g.Status == "archived" || !isAccTestName(g.Name) {
			continue
		}
		matched++
		if err := c.ArchiveDiscountGroup(ctx, g.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to archive leaked test discount group %s (%s): %s", g.ID, g.Name, err)
			continue
		}
		swept++
	}
	log.Printf("[INFO] paddle_discount_group sweeper: matched %d, swept %d", matched, swept)
	return nil
}

func sweepNotificationSettings(_ string) error {
	c := sweepClient()
	if c == nil {
		log.Printf("[WARN] paddle_notification_setting sweeper: PADDLE_API_KEY not set, skipping")
		return nil
	}
	// sweepNotificationSettings has no "already archived" skip the other
	// sweepers have — this entity has no status field at all, only a real
	// DELETE (see client.DeleteNotificationSetting), so every matching
	// object still listed is, by definition, not yet cleaned up.
	return sweepMatchingNotificationSettings(context.Background(), c, func(ns client.NotificationSetting) bool {
		return isAccTestName(ns.Description)
	})
}

// sweepMatchingNotificationSettings does the actual list-then-delete work
// behind sweepNotificationSettings, parameterized the same way
// sweepMatchingProducts is and for the identical reason — see that
// function's comment. This entity's sweep is the riskier of the two to
// get this wrong for: DELETE is destructive in a way ArchiveProduct isn't
// (a concurrent job's object doesn't just gain a status change, it
// disappears entirely, turning that job's next `Read()` into a 404 and its
// next plan into an unwanted "+create" instead of the expected no-op or
// update) — confirmed directly: an earlier version of this file's
// notification-setting verification test called sweepNotificationSettings
// itself (the broad match) from within a `TestAcc`-gated test, and a
// concurrent `pull_request`-triggered CI job's
// TestAccPaddleNotificationSetting_basic failed with exactly that "refresh
// plan was not empty... + create" symptom, because this same commit's
// concurrently-running `push`-triggered job's sweep test deleted it
// mid-run.
func sweepMatchingNotificationSettings(ctx context.Context, c *client.Client, match func(client.NotificationSetting) bool) error {
	settings, err := c.ListNotificationSettings(ctx)
	if err != nil {
		return err
	}
	var matched, swept int
	for _, ns := range settings {
		if !match(ns) {
			continue
		}
		matched++
		if err := c.DeleteNotificationSetting(ctx, ns.ID); err != nil && !client.IsNotFound(err) {
			log.Printf("[WARN] failed to delete leaked test notification setting %s (%s): %s", ns.ID, ns.Description, err)
			continue
		}
		swept++
	}
	log.Printf("[INFO] paddle_notification_setting sweeper: matched %d, swept %d", matched, swept)
	return nil
}

// TestAccSweepNotificationSettings_DeletesLeakedTestObjects is the
// real-DELETE counterpart to TestAccSweepProducts_ArchivesLeakedTestObjects
// above: sweepProducts/sweepPrices/sweepDiscounts/sweepDiscountGroups all
// share one mechanically-identical list-then-archive shape, already
// exercised end to end by the Products case, but
// sweepNotificationSettings is a genuinely different code path (list-then-
// DELETE, no "already archived" skip) that deserved its own real-sandbox
// check rather than being assumed correct by analogy. Scoped to only this
// fixture's ID (via sweepMatchingNotificationSettings, not
// sweepNotificationSettings itself) — see
// sweepMatchingNotificationSettings' comment for the concrete failure this
// scoping fixes.
func TestAccSweepNotificationSettings_DeletesLeakedTestObjects(t *testing.T) {
	testAccPreCheck(t)
	c := newTestAccClient()
	ctx := context.Background()

	leaked, err := c.CreateNotificationSetting(ctx, client.NotificationSettingCreate{
		Description:      "Acc Test Sweeper Leaked Notification Setting",
		Type:             "url",
		Destination:      "https://example.com/webhook/sweeper-leak",
		SubscribedEvents: []string{"transaction.billed"},
	})
	if err != nil {
		t.Fatalf("CreateNotificationSetting (leaked fixture): %v", err)
	}
	t.Cleanup(func() {
		_ = c.DeleteNotificationSetting(ctx, leaked.ID) // best-effort; the sweep itself is what's under test
	})

	match := func(ns client.NotificationSetting) bool { return ns.ID == leaked.ID }
	if err := sweepMatchingNotificationSettings(ctx, c, match); err != nil {
		t.Fatalf("sweepMatchingNotificationSettings: %v", err)
	}

	_, err = c.GetNotificationSetting(ctx, leaked.ID)
	if err == nil {
		t.Fatalf("notification setting %s still exists after sweep — sweep did not clean it up", leaked.ID)
	}
	if !client.IsNotFound(err) {
		t.Fatalf("GetNotificationSetting after sweep: %v", err)
	}
}

// TestSweepMatchingProducts_LogsMatchedAndSweptCount closes an honest gap
// found manually after a real -sweep run: the run completed successfully
// with zero [WARN] lines, which only proves nothing it tried to sweep
// failed — it says nothing about whether anything was actually found and
// swept, or the sweep matched zero objects, since success was previously
// silent either way. This confirms a summary line reports both numbers.
func TestSweepMatchingProducts_LogsMatchedAndSweptCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []client.Product{
					{ID: "pro_1", Name: "Acc Test A", TaxCategory: "standard"},
					{ID: "pro_2", Name: "Acc Test B", TaxCategory: "standard"},
				},
				"meta": map[string]any{"pagination": map[string]any{"has_more": false}},
			})
		case http.MethodPatch:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": client.Product{ID: "pro_1", Name: "Acc Test A", TaxCategory: "standard", Status: "archived"},
			})
		}
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	log.SetOutput(&logBuf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	c := client.New(srv.URL, "test-key")
	// Match only pro_1 — confirms "matched" and "swept" can legitimately
	// differ from the total list size, not just report len(all products).
	err := sweepMatchingProducts(context.Background(), c, func(p client.Product) bool { return p.ID == "pro_1" })
	if err != nil {
		t.Fatalf("sweepMatchingProducts: %v", err)
	}

	got := logBuf.String()
	if !strings.Contains(got, "matched 1") || !strings.Contains(got, "swept 1") {
		t.Errorf("log output = %q, want it to report matched=1 and swept=1", got)
	}
}
