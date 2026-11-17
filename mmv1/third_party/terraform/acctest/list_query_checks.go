// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// ListScopeField describes a list-query scope variable to capture from resource state.
type ListScopeField struct {
	// Name is the Terraform query variable name (and default attribute name when Attr is empty).
	Name string
	// Addr is the resource address in state when using CaptureScopesFrom.
	Addr string
	// Attr is the attribute name on the resource in state (defaults to Name when empty).
	Attr string
	// Required causes CheckState to fail when the scope attribute is unset or empty in state.
	// When false and the attribute is unset or empty, the variable is omitted from ConfigVariables
	// so `.tfquery.hcl` variables with `default = null` fall back to provider/resource defaults.
	Required bool
}

// CaptureState returns a StateCheck that captures the first non-empty candidate
// attribute value (checking AttributeValues then IdentityValues) from resourceAddr
// in JSON state for subsequent list display-name checks.
func (c *ListDisplayName) CaptureState(resourceAddr string, attrCandidates []string) statecheck.StateCheck {
	return stateCheckFunc(func(_ context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
		res, err := findStateResource(req.State, resourceAddr)
		if err != nil {
			resp.Error = err
			return
		}
		for _, k := range attrCandidates {
			if s, ok := lookupScalarStateString(res, k); ok {
				c.value = s
				return
			}
		}
		resp.Error = fmt.Errorf("no display name attribute found in state for resource %s; tried %v", resourceAddr, attrCandidates)
	})
}

// CaptureScopes returns a StateCheck that captures required and optional list
// scope variables from attributes of the same name (or field.Attr if set) on resourceAddr.
func (c *ListScopeCapture) CaptureScopes(resourceAddr string, fields []ListScopeField) statecheck.StateCheck {
	resolved := make([]ListScopeField, len(fields))
	for i, f := range fields {
		if f.Addr == "" {
			f.Addr = resourceAddr
		}
		if f.Attr == "" {
			f.Attr = f.Name
		}
		resolved[i] = f
	}
	return c.CaptureScopesFrom(resolved)
}

// CaptureScopesFrom returns a StateCheck that captures required and optional list
// scope variables from explicit resource/attribute pairs in JSON state.
func (c *ListScopeCapture) CaptureScopesFrom(fields []ListScopeField) statecheck.StateCheck {
	if c.vars == nil {
		c.vars = config.Variables{}
	}
	return stateCheckFunc(func(_ context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
		for _, field := range fields {
			attr := field.Attr
			if attr == "" {
				attr = field.Name
			}
			delete(c.vars, field.Name)
			res, err := findStateResource(req.State, field.Addr)
			if err != nil {
				resp.Error = fmt.Errorf("list scope %q: %w", field.Name, err)
				return
			}
			if s, ok := lookupScalarStateString(res, attr); ok {
				c.vars[field.Name] = config.StringVariable(s)
				continue
			}
			if field.Required {
				resp.Error = fmt.Errorf("list scope %q: attribute %q on %s is empty", field.Name, attr, field.Addr)
				return
			}
		}
	})
}

// ListIdentityCapture captures a managed resource's ResourceIdentity and identifying
// state attributes during a Config step and exposes QueryResultChecks asserting that
// `terraform query` returns a matching Identity and (when `include_resource = true`)
// a non-nil ResourceObject with matching identifying attributes.
type ListIdentityCapture struct {
	captured       bool
	resourceAddr   string
	identityChecks map[string]knownvalue.Check
	stateChecks    []querycheck.KnownValueCheck
}

// CaptureState returns a StateCheck that captures resourceAddr's IdentityValues
// and identifying AttributeValues (`id` and non-null identity attributes present in state).
func (c *ListIdentityCapture) CaptureState(resourceAddr string) statecheck.StateCheck {
	return stateCheckFunc(func(_ context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
		res, err := findStateResource(req.State, resourceAddr)
		if err != nil {
			resp.Error = err
			return
		}
		if res.IdentitySchemaVersion == nil || len(res.IdentityValues) == 0 {
			resp.Error = fmt.Errorf("%s - identity values not found in state", resourceAddr)
			return
		}

		idChecks := make(map[string]knownvalue.Check, len(res.IdentityValues))
		keys := make([]string, 0, len(res.IdentityValues))
		for k := range res.IdentityValues {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			chk, err := knownValueCheckFromJSON(res.IdentityValues[k])
			if err != nil {
				resp.Error = fmt.Errorf("%s - unsupported identity value for %q: %w", resourceAddr, k, err)
				return
			}
			idChecks[k] = chk
		}

		var objChecks []querycheck.KnownValueCheck
		added := map[string]bool{}
		for _, k := range keys {
			attrVal, ok := res.AttributeValues[k]
			if !ok || attrVal == nil || attrVal == "" {
				continue
			}
			chk, err := knownValueCheckFromJSON(attrVal)
			if err != nil {
				resp.Error = fmt.Errorf("%s - unsupported state attribute value for %q: %w", resourceAddr, k, err)
				return
			}
			objChecks = append(objChecks, querycheck.KnownValueCheck{
				Path:       tfjsonpath.New(k),
				KnownValue: chk,
			})
			added[k] = true
		}

		if len(objChecks) == 0 {
			if idVal, ok := res.AttributeValues["id"]; ok && idVal != nil && idVal != "" {
				chk, err := knownValueCheckFromJSON(idVal)
				if err != nil {
					resp.Error = fmt.Errorf("%s - unsupported state id value: %w", resourceAddr, err)
					return
				}
				objChecks = append(objChecks, querycheck.KnownValueCheck{
					Path:       tfjsonpath.New("id"),
					KnownValue: chk,
				})
			}
		}

		c.captured = true
		c.resourceAddr = resourceAddr
		c.identityChecks = idChecks
		c.stateChecks = objChecks
	})
}

// ExpectIdentity returns a QueryResultCheck asserting that at least one listed
// item at listResourceAddress has an Identity matching the captured Step 1 identity.
func (c *ListIdentityCapture) ExpectIdentity(listResourceAddress string) querycheck.QueryResultCheck {
	return queryResultCheckFunc(func(ctx context.Context, req querycheck.CheckQueryRequest, resp *querycheck.CheckQueryResponse) {
		if !c.captured || len(c.identityChecks) == 0 {
			resp.Error = fmt.Errorf("%s - identity was not captured from create step", listResourceAddress)
			return
		}
		querycheck.ExpectIdentity(listResourceAddress, c.identityChecks).CheckQuery(ctx, req, resp)
	})
}

// ExpectResourceObject returns a QueryResultCheck asserting that the listed item
// matching the captured Step 1 identity has a non-nil ResourceObject (requiring
// `include_resource = true`) whose identifying attributes match Step 1's state.
func (c *ListIdentityCapture) ExpectResourceObject(listResourceAddress string) querycheck.QueryResultCheck {
	return &listResourceObjectCheck{
		capture:             c,
		listResourceAddress: listResourceAddress,
	}
}

type listResourceObjectCheck struct {
	capture             *ListIdentityCapture
	listResourceAddress string
}

var _ querycheck.QueryResultCheck = (*listResourceObjectCheck)(nil)
var _ querycheck.QueryResultCheckWithFilters = (*listResourceObjectCheck)(nil)

func (chk *listResourceObjectCheck) QueryFilters(_ context.Context) []queryfilter.QueryFilter {
	if !chk.capture.captured || len(chk.capture.identityChecks) == 0 {
		return nil
	}
	return []queryfilter.QueryFilter{
		queryfilter.ByResourceIdentity(chk.capture.identityChecks),
	}
}

func (chk *listResourceObjectCheck) CheckQuery(ctx context.Context, req querycheck.CheckQueryRequest, resp *querycheck.CheckQueryResponse) {
	if !chk.capture.captured || len(chk.capture.identityChecks) == 0 {
		resp.Error = fmt.Errorf("%s - identity was not captured from create step", chk.listResourceAddress)
		return
	}
	// Delegate to ExpectResourceKnownValues (with filter already applied by RunQueryChecks,
	// or applied defensively below if CheckQuery is invoked directly in unit tests).
	filtered := filterQueryItemsByIdentity(ctx, req.Query, chk.listResourceAddress, chk.capture.identityChecks)
	delegatedReq := querycheck.CheckQueryRequest{
		Query:          filtered,
		QuerySummary:   req.QuerySummary,
		QuerySummaries: req.QuerySummaries,
	}
	querycheck.ExpectResourceKnownValues(
		chk.listResourceAddress,
		queryfilter.ByResourceIdentity(chk.capture.identityChecks),
		chk.capture.stateChecks,
	).CheckQuery(ctx, delegatedReq, resp)
}

func filterQueryItemsByIdentity(ctx context.Context, items []tfjson.ListResourceFoundData, listResourceAddress string, idChecks map[string]knownvalue.Check) []tfjson.ListResourceFoundData {
	filter := queryfilter.ByResourceIdentity(idChecks)
	var out []tfjson.ListResourceFoundData
	for _, item := range items {
		if strings.TrimPrefix(item.Address, "list.") != listResourceAddress {
			continue
		}
		var fResp queryfilter.FilterQueryResponse
		filter.Filter(ctx, queryfilter.FilterQueryRequest{QueryItem: item}, &fResp)
		if fResp.Include {
			out = append(out, item)
		}
	}
	return out
}

type stateCheckFunc func(context.Context, statecheck.CheckStateRequest, *statecheck.CheckStateResponse)

func (f stateCheckFunc) CheckState(ctx context.Context, req statecheck.CheckStateRequest, resp *statecheck.CheckStateResponse) {
	f(ctx, req, resp)
}

type queryResultCheckFunc func(context.Context, querycheck.CheckQueryRequest, *querycheck.CheckQueryResponse)

func (f queryResultCheckFunc) CheckQuery(ctx context.Context, req querycheck.CheckQueryRequest, resp *querycheck.CheckQueryResponse) {
	f(ctx, req, resp)
}

func findStateResource(state *tfjson.State, resourceAddr string) (*tfjson.StateResource, error) {
	if state == nil {
		return nil, fmt.Errorf("state is nil")
	}
	if state.Values == nil {
		return nil, fmt.Errorf("state does not contain any state values")
	}
	if state.Values.RootModule == nil {
		return nil, fmt.Errorf("state does not contain a root module")
	}
	if res := findStateResourceInModule(state.Values.RootModule, resourceAddr); res != nil {
		return res, nil
	}
	return nil, fmt.Errorf("resource not found in state: %s", resourceAddr)
}

func findStateResourceInModule(mod *tfjson.StateModule, resourceAddr string) *tfjson.StateResource {
	if mod == nil {
		return nil
	}
	for _, r := range mod.Resources {
		if r != nil && r.Address == resourceAddr {
			return r
		}
	}
	for _, child := range mod.ChildModules {
		if res := findStateResourceInModule(child, resourceAddr); res != nil {
			return res
		}
	}
	return nil
}

func lookupScalarStateString(res *tfjson.StateResource, key string) (string, bool) {
	if res == nil {
		return "", false
	}
	if res.AttributeValues != nil {
		if s, ok := scalarToString(res.AttributeValues[key]); ok {
			return s, true
		}
	}
	if res.IdentityValues != nil {
		if s, ok := scalarToString(res.IdentityValues[key]); ok {
			return s, true
		}
	}
	return "", false
}

func scalarToString(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch val := v.(type) {
	case string:
		return val, val != ""
	case json.Number:
		s := val.String()
		return s, s != ""
	case bool:
		return strconv.FormatBool(val), true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case float64:
		if math.Trunc(val) == val {
			return strconv.FormatInt(int64(val), 10), true
		}
		return strconv.FormatFloat(val, 'f', -1, 64), true
	default:
		return "", false
	}
}

func knownValueCheckFromJSON(v any) (knownvalue.Check, error) {
	if v == nil {
		return knownvalue.Null(), nil
	}
	switch val := v.(type) {
	case string:
		return knownvalue.StringExact(val), nil
	case bool:
		return knownvalue.Bool(val), nil
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return knownvalue.Int64Exact(i), nil
		}
		bf, _, err := big.ParseFloat(val.String(), 10, 512, big.ToNearestEven)
		if err != nil {
			return nil, fmt.Errorf("invalid json.Number %q: %w", val.String(), err)
		}
		return knownvalue.NumberExact(bf), nil
	case int:
		return knownvalue.Int64Exact(int64(val)), nil
	case int64:
		return knownvalue.Int64Exact(val), nil
	case float64:
		if !math.IsNaN(val) && !math.IsInf(val, 0) && math.Trunc(val) == val && val >= math.MinInt64 && val < float64(math.MaxInt64) {
			return knownvalue.Int64Exact(int64(val)), nil
		}
		return knownvalue.NumberExact(new(big.Float).SetPrec(512).SetFloat64(val)), nil
	case []any:
		elemChecks := make([]knownvalue.Check, len(val))
		for i, elem := range val {
			chk, err := knownValueCheckFromJSON(elem)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			elemChecks[i] = chk
		}
		return knownvalue.ListExact(elemChecks), nil
	case map[string]any:
		objChecks := make(map[string]knownvalue.Check, len(val))
		for k, elem := range val {
			chk, err := knownValueCheckFromJSON(elem)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", k, err)
			}
			objChecks[k] = chk
		}
		return knownvalue.ObjectExact(objChecks), nil
	default:
		return nil, fmt.Errorf("unsupported JSON value type %T", v)
	}
}
