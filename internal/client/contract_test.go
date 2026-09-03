package client

// Forward-only contract check against Paddle's own vendored OpenAPI spec
// (third_party/paddle-openapi/, see that directory's README for the
// pinned source commit and refresh instructions).
//
// "Forward-only" means: for every JSON field this provider's client
// structs actually read or write, does that field still exist in
// Paddle's real API at the same nested path, with a structurally
// compatible shape (object vs. array vs. scalar — not exact scalar
// subtype, and not requiredness)? It deliberately does NOT check the
// reverse direction (does the provider model every field the spec has) —
// this provider deliberately leaves many fields unmodeled (there's no
// paddle_address resource, for instance), so a bidirectional diff would
// constantly flag deliberate scope decisions as "drift." Forward-only
// targets the actual repeated bug class in this project's history
// instead: a hand-written Go struct silently drifting from what Paddle's
// API really returns/accepts (TransactionItem's nested price.id modeled
// as a flat price_id, NextTransactionPreview's items read from a
// non-existent top-level key — both real, previously-shipped bugs this
// check would have caught).
//
// This lives inside package client (not a separate tools/ binary)
// specifically so it can reach unexported wire types too (e.g.
// nextTransactionPreviewWire) — those are exactly the types most likely
// to have hand-modeling mistakes, since they exist only to work around a
// shape Paddle's JSON doesn't match cleanly.
//
// Extending coverage: add one entry to contractManifest. Nested fields
// need no entry of their own — they're walked automatically from their
// parent's Go type and matched structurally against the spec, not by
// name.

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// contractManifest maps a Go client type this provider actually uses to
// decode/encode Paddle API JSON to the OpenAPI schema name
// (components.schemas.<name>) that describes the same object on Paddle's
// side. Only top-level entries need naming — nested fields (e.g.
// Price.BillingCycle) are matched structurally via the spec's own
// properties/$ref chain, whatever that nested schema happens to be
// called upstream.
var contractManifest = []struct {
	name   string
	goType reflect.Type
	schema string
}{
	{"Product", reflect.TypeOf(Product{}), "Product"},
	{"Price", reflect.TypeOf(Price{}), "Price"},
	{"Discount", reflect.TypeOf(Discount{}), "Discount"},
	{"DiscountGroup", reflect.TypeOf(DiscountGroup{}), "DiscountGroup"},
	{"NotificationSetting", reflect.TypeOf(NotificationSetting{}), "NotificationSetting"},
	{"Subscription", reflect.TypeOf(Subscription{}), "Subscription"},
	{"Transaction", reflect.TypeOf(Transaction{}), "Transaction"},
	{"TransactionItem", reflect.TypeOf(TransactionItem{}), "TransactionItem"},
	{"Adjustment", reflect.TypeOf(Adjustment{}), "Adjustment"},
	{"AdjustmentItem", reflect.TypeOf(AdjustmentItem{}), "AdjustmentItem"},
	{"Customer", reflect.TypeOf(Customer{}), "Customer"},
	{"Address", reflect.TypeOf(Address{}), "Address"},
	{"Notification", reflect.TypeOf(Notification{}), "Notification"},
	{"NotificationLog", reflect.TypeOf(NotificationLog{}), "NotificationLog"},
	{"Event", reflect.TypeOf(Event{}), "Event"},
	// nextTransactionPreviewWire is unexported — the exact reason this
	// check lives inside package client. This is the real wire shape
	// NextTransactionPreview.UnmarshalJSON decodes into, and the one that
	// was wrong in production once already (items read from a top-level
	// "items" key that doesn't exist — the real data is nested under
	// SubscriptionNextTransaction.details.line_items).
	{"nextTransactionPreviewWire", reflect.TypeOf(nextTransactionPreviewWire{}), "SubscriptionNextTransaction"},
}

// contractIgnore lists Go field paths (dot-separated, matching the path
// contractWalk builds) that are known, deliberate differences from the
// spec — never add an entry here to silence a failure without first
// confirming it's deliberate, not a real bug. Keyed by manifest entry
// name.
var contractIgnore = map[string]map[string]bool{
	"Adjustment": {
		// tax_mode is real (components.schemas.AdjustmentTaxMode via
		// AdjustmentCreate), but writeOnly: true there and genuinely
		// absent from the Adjustment response entity — confirmed by
		// reading both schemas directly. Adjustment deliberately reuses
		// one Go struct for both the create request and decoding
		// responses (see its doc comment), so this field exists on the
		// Go side for the request half only; there's no response-side
		// schema it could ever match.
		"tax_mode": true,
	},
}

func TestContractAgainstPaddleOpenAPI(t *testing.T) {
	spec := loadPaddleSpec(t)

	for _, entry := range contractManifest {
		entry := entry
		t.Run(entry.name, func(t *testing.T) {
			root := resolveSpecSchema(t, spec, entry.schema)
			ignore := contractIgnore[entry.name]

			var mismatches []string
			walkGoType(entry.goType, "", func(path string, kind reflect.Kind, elem reflect.Type) {
				if ignore[path] {
					return
				}
				specNode, found := resolveSpecPath(spec, root, path)
				if !found {
					mismatches = append(mismatches, fmt.Sprintf("%s: field not found in Paddle's OpenAPI spec at components.schemas.%s (Go type: %s)", path, entry.schema, entry.goType))
					return
				}
				specKind := specNodeKind(spec, specNode)
				goKind := normalizeGoKind(kind)
				if !kindsCompatible(goKind, specKind) {
					mismatches = append(mismatches, fmt.Sprintf("%s: Go models this as %s, Paddle's spec says %s (components.schemas.%s)", path, goKind, specKind, entry.schema))
				}
			})

			sort.Strings(mismatches)
			for _, m := range mismatches {
				t.Error(m)
			}
		})
	}
}

// --- Go-side: walk a client struct's JSON-tagged fields ---

// walkGoType recursively visits every JSON-tagged field reachable from t
// (a struct type), calling visit with the dotted field path, the field's
// effective kind, and (for struct/slice-of-struct fields) the element
// type to recurse into. Pointers are transparently dereferenced — this
// provider uses *T for nullable fields, which the spec expresses as
// "anyOf: [<type>, null]", not a distinct kind.
func walkGoType(t reflect.Type, prefix string, visit func(path string, kind reflect.Kind, elem reflect.Type)) {
	walkGoTypeDepth(t, prefix, visit, 0)
}

func walkGoTypeDepth(t reflect.Type, prefix string, visit func(path string, kind reflect.Kind, elem reflect.Type), depth int) {
	if depth > 8 {
		return // guard against unexpected recursion; no entity here nests this deep
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported field
		}
		jsonTag := f.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		name := strings.Split(jsonTag, ",")[0]
		if name == "" {
			continue // untagged field: nothing on the wire to check
		}

		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}

		switch ft.Kind() {
		case reflect.Struct:
			visit(path, reflect.Struct, ft)
			walkGoTypeDepth(ft, path, visit, depth+1)
		case reflect.Slice, reflect.Array:
			elem := ft.Elem()
			for elem.Kind() == reflect.Pointer {
				elem = elem.Elem()
			}
			visit(path, reflect.Slice, elem)
			if elem.Kind() == reflect.Struct {
				walkGoTypeDepth(elem, path, visit, depth+1)
			}
		default:
			visit(path, ft.Kind(), nil)
		}
	}
}

// normalizeGoKind maps a reflect.Kind to the same small vocabulary
// specNodeKind uses for the spec side, so the two are directly
// comparable.
func normalizeGoKind(k reflect.Kind) string {
	switch k {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Struct:
		return "object"
	case reflect.Slice, reflect.Array:
		return "array"
	case reflect.Map, reflect.Interface:
		return "any" // e.g. CustomData map[string]any — existence-checked, not type-checked
	default:
		return "unknown"
	}
}

// kindsCompatible reports whether a Go-side kind and a spec-side kind are
// close enough that mismatching them would indicate real drift (a
// flat-vs-nested shape change, a scalar vs. object change) rather than
// noise. "any" (an untyped Go blob like CustomData) accepts anything —
// its whole point is Paddle can put arbitrary shapes there. "integer" and
// "number" are treated as interchangeable: Paddle's spec isn't always
// consistent about which numeric JSON Schema type it uses for what this
// provider models as int64, and that distinction isn't the bug class this
// check targets.
func kindsCompatible(goKind, specKind string) bool {
	if goKind == "any" || specKind == "any" {
		return true
	}
	if goKind == specKind {
		return true
	}
	if (goKind == "integer" || goKind == "number") && (specKind == "integer" || specKind == "number") {
		return true
	}
	return false
}

// --- Spec-side: load and navigate the vendored OpenAPI document ---

func loadPaddleSpec(t *testing.T) map[string]any {
	t.Helper()
	path := paddleSpecPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading vendored Paddle OpenAPI spec at %s: %v (see third_party/paddle-openapi/README.md)", path, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing vendored Paddle OpenAPI spec: %v", err)
	}
	return doc
}

// paddleSpecPath locates third_party/paddle-openapi/v1/openapi.yaml
// relative to this source file, not the working directory — `go test`
// runs with cwd set to the package directory, but callers/tools invoking
// this differently (e.g. `go test ./...` from repo root, which is the
// normal case, or an IDE running a single test) should still find it.
func paddleSpecPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine this test file's own path via runtime.Caller")
	}
	// this file is internal/client/contract_test.go; the spec is at
	// third_party/paddle-openapi/v1/openapi.yaml, three directories up.
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "third_party", "paddle-openapi", "v1", "openapi.yaml")
}

// resolveSpecSchema returns the fully-dereferenced node for
// components.schemas.<name>, failing the test immediately (not per-field)
// if the name itself doesn't exist — that's a manifest bug, not drift to
// report field-by-field.
func resolveSpecSchema(t *testing.T, spec map[string]any, name string) map[string]any {
	t.Helper()
	components, _ := spec["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	node, ok := schemas[name].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.%s does not exist in the vendored Paddle OpenAPI spec — manifest entry is stale, fix contractManifest", name)
	}
	return resolveSchema(spec, node)
}

// resolveSchema fully dereferences $ref/allOf/anyOf down to a node with a
// concrete "type" (or "properties", for objects that omit the redundant
// "type: object"). allOf is treated as a shallow merge (Paddle's spec
// uses it to attach readOnly/default on top of a single $ref, never to
// combine genuinely different object shapes here). anyOf is resolved by
// dropping the "type: null" nullability variant and recursing into
// whatever's left — this provider's own nullable fields are *T pointers,
// which walkGoType already unwraps the same way.
func resolveSchema(spec map[string]any, node map[string]any) map[string]any {
	for i := 0; i < 20; i++ { // guard against a $ref cycle; the real spec has none
		if ref, ok := node["$ref"].(string); ok {
			node = followRef(spec, ref)
			continue
		}
		if allOf, ok := node["allOf"].([]any); ok && len(allOf) > 0 {
			merged := map[string]any{}
			for _, item := range allOf {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				resolved := resolveSchema(spec, m)
				for k, v := range resolved {
					merged[k] = v
				}
			}
			node = merged
			continue
		}
		if anyOf, ok := node["anyOf"].([]any); ok && len(anyOf) > 0 {
			var chosen map[string]any
			for _, item := range anyOf {
				m, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if t, _ := m["type"].(string); t == "null" {
					continue
				}
				chosen = m
				break
			}
			if chosen == nil {
				chosen, _ = anyOf[0].(map[string]any)
			}
			if chosen == nil {
				break
			}
			node = resolveSchema(spec, chosen)
			continue
		}
		break
	}
	return node
}

func followRef(spec map[string]any, ref string) map[string]any {
	parts := strings.Split(strings.TrimPrefix(ref, "#/"), "/")
	var cur any = spec
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return map[string]any{}
		}
		cur = m[p]
	}
	m, _ := cur.(map[string]any)
	return m
}

// resolveSpecPath walks path (a dot-separated field path matching what
// walkGoType builds) from root, descending into array "items" schemas
// transparently wherever the current node is an array — the same way a
// Go []T field's own nested fields are addressed without an index
// segment in the path.
func resolveSpecPath(spec map[string]any, root map[string]any, path string) (map[string]any, bool) {
	node := root
	for _, segment := range strings.Split(path, ".") {
		if specNodeKind(spec, node) == "array" {
			items, ok := node["items"].(map[string]any)
			if !ok {
				return nil, false
			}
			node = resolveSchema(spec, items)
		}
		props, _ := node["properties"].(map[string]any)
		next, ok := props[segment].(map[string]any)
		if !ok {
			return nil, false
		}
		node = resolveSchema(spec, next)
	}
	return node, true
}

// specNodeKind classifies an already-resolved spec node into the same
// small vocabulary normalizeGoKind uses.
func specNodeKind(spec map[string]any, node map[string]any) string {
	if t, ok := node["type"].(string); ok {
		switch t {
		case "integer":
			return "integer"
		case "number":
			return "number"
		case "string":
			return "string"
		case "boolean":
			return "boolean"
		case "array":
			return "array"
		case "object":
			if _, hasProps := node["properties"]; !hasProps {
				return "any" // schemaless object, e.g. CustomData's unevaluatedProperties blob
			}
			return "object"
		}
	}
	if _, ok := node["properties"]; ok {
		return "object"
	}
	if _, ok := node["items"]; ok {
		return "array"
	}
	return "any"
}
