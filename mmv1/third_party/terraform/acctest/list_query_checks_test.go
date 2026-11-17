// Copyright IBM Corp. 2014, 2026
// SPDX-License-Identifier: MPL-2.0

package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/querycheck"
	"github.com/hashicorp/terraform-plugin-testing/querycheck/queryfilter"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func makeTestJSONState(addr string, attrs, identity map[string]any) *tfjson.State {
	var idVer *uint64
	if identity != nil {
		idVer = uint64Ptr(1)
	}
	return &tfjson.State{
		FormatVersion: "1.0",
		Values: &tfjson.StateValues{
			RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{
						Address:               addr,
						AttributeValues:       attrs,
						IdentitySchemaVersion: idVer,
						IdentityValues:        identity,
					},
				},
			},
		},
	}
}

func TestListDisplayNameCaptureState(t *testing.T) {
	t.Parallel()

	t.Run("captures first non-empty candidate including json.Number and identity fallback", func(t *testing.T) {
		t.Parallel()
		state := makeTestJSONState(
			"google_compute_network_firewall_policy_packet_mirroring_rule.primary",
			map[string]any{
				"display_name": "",
				"name":         nil,
			},
			map[string]any{
				"priority": json.Number("1000"),
			},
		)

		var dn ListDisplayName
		var resp statecheck.CheckStateResponse
		dn.CaptureState(
			"google_compute_network_firewall_policy_packet_mirroring_rule.primary",
			[]string{"display_name", "name", "priority"},
		).CheckState(context.Background(), statecheck.CheckStateRequest{State: state}, &resp)
		if resp.Error != nil {
			t.Fatalf("unexpected CaptureState error: %v", resp.Error)
		}
		if err := dn.CheckValue().CheckValue("1000"); err != nil {
			t.Fatalf("expected CheckValue(\"1000\") to pass, got: %v", err)
		}
	})

	t.Run("errors when all candidates empty or resource missing", func(t *testing.T) {
		t.Parallel()
		state := makeTestJSONState("google_example.primary", map[string]any{"name": ""}, nil)

		var dn ListDisplayName
		var resp statecheck.CheckStateResponse
		dn.CaptureState("google_example.primary", []string{"display_name", "name"}).CheckState(
			context.Background(),
			statecheck.CheckStateRequest{State: state},
			&resp,
		)
		if resp.Error == nil {
			t.Fatal("expected error when no display name candidate is non-empty")
		}
	})
}

func TestListScopeCaptureScopes(t *testing.T) {
	t.Parallel()

	t.Run("captures required, optional, identity-fallback, and omits null/empty optional scopes", func(t *testing.T) {
		t.Parallel()
		state := makeTestJSONState(
			"google_example.primary",
			map[string]any{
				"location":      "us-east1",
				"project":       "custom-project",
				"region":        nil, // e.g. url_param_only / ignore_read unset in AttributeValues
				"collection_id": "",  // optional unset scope -> omitted so .tfquery.hcl default = null applies
			},
			map[string]any{
				"zone": "us-east1-b", // fallback from IdentityValues when absent from AttributeValues
			},
		)

		var scope ListScopeCapture
		var resp statecheck.CheckStateResponse
		scope.CaptureScopes("google_example.primary", []ListScopeField{
			{Name: "location", Required: true},
			{Name: "project", Required: false},
			{Name: "region", Required: false},
			{Name: "zone", Required: false},
			{Name: "collection_id", Required: false},
		}).CheckState(context.Background(), statecheck.CheckStateRequest{State: state}, &resp)
		if resp.Error != nil {
			t.Fatalf("unexpected CaptureScopes error: %v", resp.Error)
		}

		vars := scope.AsConfigVariables()
		raw, err := json.Marshal(vars)
		if err != nil {
			t.Fatalf("failed to marshal ConfigVariables: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("failed to unmarshal ConfigVariables: %v", err)
		}

		if got["location"] != "us-east1" || got["project"] != "custom-project" || got["zone"] != "us-east1-b" {
			t.Fatalf("unexpected captured variables: %#v", got)
		}
		if _, exists := got["region"]; exists {
			t.Fatalf("expected unset optional scope 'region' to be omitted, got: %#v", got)
		}
		if _, exists := got["collection_id"]; exists {
			t.Fatalf("expected empty optional scope 'collection_id' to be omitted, got: %#v", got)
		}
	})

	t.Run("errors when required scope is empty", func(t *testing.T) {
		t.Parallel()
		state := makeTestJSONState("google_example.primary", map[string]any{"location": ""}, nil)

		var scope ListScopeCapture
		var resp statecheck.CheckStateResponse
		scope.CaptureScopes("google_example.primary", []ListScopeField{
			{Name: "location", Required: true},
		}).CheckState(context.Background(), statecheck.CheckStateRequest{State: state}, &resp)
		if resp.Error == nil {
			t.Fatal("expected error when required scope is empty")
		}
	})
}

func TestListIdentityCaptureUnit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	state := makeTestJSONState(
		"google_example_rule.primary",
		map[string]any{
			"id":              "projects/my-proj/global/firewallPolicies/my-pol/packetMirroringRules/1000",
			"project":         "my-proj",
			"firewall_policy": "my-pol",
			"priority":        json.Number("1000"),
			"optional_tag":    nil,
		},
		map[string]any{
			"project":         "my-proj",
			"firewall_policy": "my-pol",
			"priority":        json.Number("1000"),
			"optional_tag":    nil,
		},
	)

	var cap ListIdentityCapture

	t.Run("fails before CaptureState runs", func(t *testing.T) {
		var qResp querycheck.CheckQueryResponse
		cap.ExpectIdentity("google_example_rule.list_query").CheckQuery(ctx, querycheck.CheckQueryRequest{}, &qResp)
		if qResp.Error == nil {
			t.Fatal("expected error when CaptureState has not run")
		}
	})

	var sResp statecheck.CheckStateResponse
	cap.CaptureState("google_example_rule.primary").CheckState(ctx, statecheck.CheckStateRequest{State: state}, &sResp)
	if sResp.Error != nil {
		t.Fatalf("CaptureState failed: %v", sResp.Error)
	}

	matchingIdentity := map[string]any{
		"project":         "my-proj",
		"firewall_policy": "my-pol",
		"priority":        json.Number("1000"),
		"optional_tag":    nil,
	}
	otherIdentity := map[string]any{
		"project":         "my-proj",
		"firewall_policy": "my-pol",
		"priority":        json.Number("2000"),
		"optional_tag":    "v1",
	}

	t.Run("passes ExpectIdentity and ExpectResourceObject when matching item has ResourceObject", func(t *testing.T) {
		req := querycheck.CheckQueryRequest{
			Query: []tfjson.ListResourceFoundData{
				{
					Address:  "list.google_example_rule.list_query",
					Identity: otherIdentity,
					ResourceObject: map[string]any{
						"project":         "my-proj",
						"firewall_policy": "my-pol",
						"priority":        json.Number("2000"),
					},
				},
				{
					Address:  "list.google_example_rule.list_query",
					Identity: matchingIdentity,
					ResourceObject: map[string]any{
						"project":         "my-proj",
						"firewall_policy": "my-pol",
						"priority":        json.Number("1000"),
						"description":     "listed item",
					},
				},
			},
		}

		var idResp querycheck.CheckQueryResponse
		cap.ExpectIdentity("google_example_rule.list_query").CheckQuery(ctx, req, &idResp)
		if idResp.Error != nil {
			t.Fatalf("ExpectIdentity failed: %v", idResp.Error)
		}

		var objResp querycheck.CheckQueryResponse
		cap.ExpectResourceObject("google_example_rule.list_query").CheckQuery(ctx, req, &objResp)
		if objResp.Error != nil {
			t.Fatalf("ExpectResourceObject failed: %v", objResp.Error)
		}
	})

	t.Run("fails ExpectResourceObject when include_resource was omitted (ResourceObject == nil)", func(t *testing.T) {
		req := querycheck.CheckQueryRequest{
			Query: []tfjson.ListResourceFoundData{
				{
					Address:        "list.google_example_rule.list_query",
					Identity:       matchingIdentity,
					ResourceObject: nil,
				},
			},
		}
		var objResp querycheck.CheckQueryResponse
		cap.ExpectResourceObject("google_example_rule.list_query").CheckQuery(ctx, req, &objResp)
		if objResp.Error == nil || !strings.Contains(objResp.Error.Error(), "include_resource") {
			t.Fatalf("expected include_resource error when ResourceObject is nil, got: %v", objResp.Error)
		}
	})

	t.Run("fails ExpectResourceObject when ResourceObject attribute diverges from state", func(t *testing.T) {
		req := querycheck.CheckQueryRequest{
			Query: []tfjson.ListResourceFoundData{
				{
					Address:  "list.google_example_rule.list_query",
					Identity: matchingIdentity,
					ResourceObject: map[string]any{
						"project":         "wrong-proj",
						"firewall_policy": "my-pol",
						"priority":        json.Number("1000"),
					},
				},
			},
		}
		var objResp querycheck.CheckQueryResponse
		cap.ExpectResourceObject("google_example_rule.list_query").CheckQuery(ctx, req, &objResp)
		if objResp.Error == nil {
			t.Fatal("expected error when ResourceObject attribute mismatches Step 1 state")
		}
	})

	t.Run("fails ExpectIdentity when optional null identity key is missing", func(t *testing.T) {
		req := querycheck.CheckQueryRequest{
			Query: []tfjson.ListResourceFoundData{
				{
					Address: "list.google_example_rule.list_query",
					Identity: map[string]any{
						"project":         "my-proj",
						"firewall_policy": "my-pol",
						"priority":        json.Number("1000"),
					},
				},
			},
		}
		var idResp querycheck.CheckQueryResponse
		cap.ExpectIdentity("google_example_rule.list_query").CheckQuery(ctx, req, &idResp)
		if idResp.Error == nil {
			t.Fatal("expected error when optional identity key is missing from listed identity")
		}
	})
}

func TestListQueryChecks_EndToEndUnitTestCLI(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skipf("terraform CLI not found on PATH: %v", err)
	}

	UnsetTestProviderConfigEnvs(t)

	type storedJob struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Schedule    string         `json:"schedule"`
		TimeZone    string         `json:"timeZone"`
		State       string         `json:"state"`
		HttpTarget  map[string]any `json:"httpTarget"`
	}

	var mu sync.Mutex
	jobs := map[string]storedJob{}
	var listPaths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/v1/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		// Expected paths:
		// POST/GET projects/{p}/locations/{loc}/jobs
		// GET/DELETE projects/{p}/locations/{loc}/jobs/{id}
		if len(parts) < 5 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "jobs" {
			http.Error(w, fmt.Sprintf("unexpected path: %s", r.URL.Path), http.StatusNotFound)
			return
		}
		proj, loc := parts[1], parts[3]

		mu.Lock()
		defer mu.Unlock()

		switch {
		case r.Method == http.MethodPost && len(parts) == 5:
			var reqBody storedJob
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			reqBody.State = "ENABLED"
			jobs[reqBody.Name] = reqBody
			_ = json.NewEncoder(w).Encode(reqBody)

		case r.Method == http.MethodPost && len(parts) == 6 && (strings.HasSuffix(parts[5], ":resume") || strings.HasSuffix(parts[5], ":pause")):
			jobID := strings.SplitN(parts[5], ":", 2)[0]
			fullName := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", proj, loc, jobID)
			j, ok := jobs[fullName]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(j)

		case r.Method == http.MethodGet && len(parts) == 6:
			fullName := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", proj, loc, parts[5])
			j, ok := jobs[fullName]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":404,"status":"NOT_FOUND"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(j)

		case r.Method == http.MethodGet && len(parts) == 5:
			listPaths = append(listPaths, path)
			prefix := fmt.Sprintf("projects/%s/locations/%s/jobs/", proj, loc)
			items := []storedJob{
				{
					Name:        prefix + "decoy-job",
					Description: "decoy",
					Schedule:    "0 * * * *",
					TimeZone:    "Etc/UTC",
					State:       "ENABLED",
					HttpTarget:  map[string]any{"uri": "https://example.com/decoy", "httpMethod": "GET"},
				},
			}
			for k, v := range jobs {
				if strings.HasPrefix(k, prefix) {
					items = append(items, v)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jobs": items})

		case r.Method == http.MethodDelete && len(parts) == 6:
			fullName := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", proj, loc, parts[5])
			delete(jobs, fullName)
			_, _ = w.Write([]byte(`{}`))

		default:
			http.Error(w, fmt.Sprintf("unsupported %s %s", r.Method, r.URL.Path), http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	endpoint := srv.URL + "/v1/"

	t.Run("explicit_optional_scopes_override_provider_defaults", func(t *testing.T) {
		var listDisplayName ListDisplayName
		var listScope ListScopeCapture
		var listIdentity ListIdentityCapture

		resource.UnitTest(t, resource.TestCase{
			TerraformVersionChecks: []tfversion.TerraformVersionCheck{
				tfversion.SkipBelow(tfversion.Version1_14_0),
			},
			ProtoV5ProviderFactories: ProtoV5ProviderFactories(t),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
provider "google" {
  access_token                    = "fake-token"
  project                         = "default-proj"
  region                          = "us-central1"
  cloud_scheduler_custom_endpoint = %q
}

resource "google_cloud_scheduler_job" "job" {
  name        = "explicit-scope-job"
  project     = "override-proj"
  region      = "europe-west1"
  description = "test job"
  schedule    = "*/5 * * * *"
  time_zone   = "Etc/UTC"
  http_target {
    http_method = "GET"
    uri         = "https://example.com/"
  }
}
`, endpoint),
					ConfigStateChecks: []statecheck.StateCheck{
						listDisplayName.CaptureState("google_cloud_scheduler_job.job", []string{"name"}),
						listScope.CaptureScopes("google_cloud_scheduler_job.job", []ListScopeField{
							{Name: "region", Required: false},
							{Name: "project", Required: false},
						}),
						listIdentity.CaptureState("google_cloud_scheduler_job.job"),
					},
				},
				{
					Query: true,
					Config: `
variable "region" {
  type    = string
  default = null
}
variable "project" {
  type    = string
  default = null
}

list "google_cloud_scheduler_job" "list_query" {
  provider         = google
  include_resource = true
  limit            = 100
  config {
    region  = var.region
    project = var.project
  }
}
`,
					ConfigVariables: listScope.AsConfigVariables(),
					QueryResultChecks: []querycheck.QueryResultCheck{
						querycheck.ExpectLengthAtLeast("google_cloud_scheduler_job.list_query", 2),
						querycheck.ExpectResourceDisplayName(
							"google_cloud_scheduler_job.list_query",
							queryfilter.ByDisplayName(listDisplayName.CheckValue()),
							listDisplayName.CheckValue(),
						),
						listIdentity.ExpectIdentity("google_cloud_scheduler_job.list_query"),
						listIdentity.ExpectResourceObject("google_cloud_scheduler_job.list_query"),
					},
				},
			},
		})

		mu.Lock()
		gotPaths := append([]string(nil), listPaths...)
		mu.Unlock()
		wantPath := "projects/override-proj/locations/europe-west1/jobs"
		found := false
		for _, p := range gotPaths {
			if p == wantPath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected list request to hit %q, got %v", wantPath, gotPaths)
		}
	})

	t.Run("null_optional_scope_variable_falls_back_to_provider_default", func(t *testing.T) {
		var listDisplayName ListDisplayName
		var listScope ListScopeCapture
		var listIdentity ListIdentityCapture

		resource.UnitTest(t, resource.TestCase{
			TerraformVersionChecks: []tfversion.TerraformVersionCheck{
				tfversion.SkipBelow(tfversion.Version1_14_0),
			},
			ProtoV5ProviderFactories: ProtoV5ProviderFactories(t),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
provider "google" {
  access_token                    = "fake-token"
  project                         = "default-proj"
  region                          = "us-east4"
  cloud_scheduler_custom_endpoint = %q
}

resource "google_cloud_scheduler_job" "job" {
  name        = "default-scope-job"
  description = "test default scope"
  schedule    = "*/5 * * * *"
  time_zone   = "Etc/UTC"
  http_target {
    http_method = "GET"
    uri         = "https://example.com/"
  }
}
`, endpoint),
					ConfigStateChecks: []statecheck.StateCheck{
						listDisplayName.CaptureState("google_cloud_scheduler_job.job", []string{"name"}),
						// Simulate an unset optional attribute in state (`Attr: "nonexistent_optional_region"`)
						// so var.region in .tfquery.hcl receives `default = null` and falls back to provider default.
						listScope.CaptureScopes("google_cloud_scheduler_job.job", []ListScopeField{
							{Name: "region", Attr: "nonexistent_optional_region", Required: false},
							{Name: "project", Attr: "nonexistent_optional_project", Required: false},
						}),
						listIdentity.CaptureState("google_cloud_scheduler_job.job"),
					},
				},
				{
					Query: true,
					Config: `
variable "region" {
  type    = string
  default = null
}
variable "project" {
  type    = string
  default = null
}

list "google_cloud_scheduler_job" "list_query" {
  provider         = google
  include_resource = true
  limit            = 100
  config {
    region  = var.region
    project = var.project
  }
}
`,
					ConfigVariables: listScope.AsConfigVariables(),
					QueryResultChecks: []querycheck.QueryResultCheck{
						querycheck.ExpectLengthAtLeast("google_cloud_scheduler_job.list_query", 2),
						listIdentity.ExpectIdentity("google_cloud_scheduler_job.list_query"),
						listIdentity.ExpectResourceObject("google_cloud_scheduler_job.list_query"),
					},
				},
			},
		})

		mu.Lock()
		gotPaths := append([]string(nil), listPaths...)
		mu.Unlock()
		wantPath := "projects/default-proj/locations/us-east4/jobs"
		found := false
		for _, p := range gotPaths {
			if p == wantPath {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected null var.region/var.project to fall back to provider defaults (%q), got %v", wantPath, gotPaths)
		}
	})
}
