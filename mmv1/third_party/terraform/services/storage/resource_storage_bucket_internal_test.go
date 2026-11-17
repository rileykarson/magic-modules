package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/storage/v1"
)

func TestLabelDiffSuppress(t *testing.T) {
	cases := map[string]struct {
		K, Old, New        string
		ExpectDiffSuppress bool
	}{
		"missing goog-dataplex-asset-id": {
			K:                  "labels.goog-dataplex-asset-id",
			Old:                "test-bucket",
			New:                "",
			ExpectDiffSuppress: true,
		},
		"explicit goog-dataplex-asset-id": {
			K:                  "labels.goog-dataplex-asset-id",
			Old:                "test-bucket",
			New:                "test-bucket-1",
			ExpectDiffSuppress: false,
		},
		"missing goog-dataplex-lake-id": {
			K:                  "labels.goog-dataplex-lake-id",
			Old:                "test-lake",
			New:                "",
			ExpectDiffSuppress: true,
		},
		"explicit goog-dataplex-lake-id": {
			K:                  "labels.goog-dataplex-lake-id",
			Old:                "test-lake",
			New:                "test-lake-1",
			ExpectDiffSuppress: false,
		},
		"missing goog-dataplex-project-id": {
			K:                  "labels.goog-dataplex-project-id",
			Old:                "test-project-12345",
			New:                "",
			ExpectDiffSuppress: true,
		},
		"explicit goog-dataplex-project-id": {
			K:                  "labels.goog-dataplex-project-id",
			Old:                "test-project-12345",
			New:                "test-project-12345-1",
			ExpectDiffSuppress: false,
		},
		"missing goog-dataplex-zone-id": {
			K:                  "labels.goog-dataplex-zone-id",
			Old:                "test-zone1",
			New:                "",
			ExpectDiffSuppress: true,
		},
		"explicit goog-dataplex-zone-id": {
			K:                  "labels.goog-dataplex-zone-id",
			Old:                "test-zone1",
			New:                "test-zone1-1",
			ExpectDiffSuppress: false,
		},
		"labels.%": {
			K:                  "labels.%",
			Old:                "5",
			New:                "1",
			ExpectDiffSuppress: true,
		},
		"deleted custom key": {
			K:                  "labels.my-label",
			Old:                "my-value",
			New:                "",
			ExpectDiffSuppress: false,
		},
		"added custom key": {
			K:                  "labels.my-label",
			Old:                "",
			New:                "my-value",
			ExpectDiffSuppress: false,
		},
	}
	for tn, tc := range cases {
		if resourceDataplexLabelDiffSuppress(tc.K, tc.Old, tc.New, nil) != tc.ExpectDiffSuppress {
			t.Errorf("bad: %s, %q: %q => %q expect DiffSuppress to return %t", tn, tc.K, tc.Old, tc.New, tc.ExpectDiffSuppress)
		}
	}
}

func TestIsIgnorableStorageObjectDeleteError(t *testing.T) {
	cases := map[string]struct {
		err      error
		expected bool
	}{
		// Callers check err != nil before calling, so nil is not an ignorable error.
		"nil": {
			err:      nil,
			expected: false,
		},
		"404 no such object": {
			err:      &googleapi.Error{Code: 404, Message: "No such object: example-bucket/path/to/object"},
			expected: true,
		},
		"410 gone": {
			err:      &googleapi.Error{Code: 410, Message: "Gone"},
			expected: true,
		},
		"wrapped 404": {
			err:      fmt.Errorf("deleting object: %w", &googleapi.Error{Code: 404, Message: "No such object: example-bucket/path/to/object"}),
			expected: true,
		},
		"double wrapped 404": {
			err:      fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", &googleapi.Error{Code: 404, Message: "No such object"})),
			expected: true,
		},
		"errors.Join with 404": {
			err:      errors.Join(errors.New("network blip"), &googleapi.Error{Code: 404, Message: "No such object"}),
			expected: true,
		},
		"403 forbidden": {
			err:      &googleapi.Error{Code: 403, Message: "Forbidden"},
			expected: false,
		},
		"409 conflict": {
			err:      &googleapi.Error{Code: 409, Message: "Conflict"},
			expected: false,
		},
		"400 bad request": {
			err:      &googleapi.Error{Code: 400, Message: "Bad Request"},
			expected: false,
		},
		"429 rate limit": {
			err:      &googleapi.Error{Code: 429, Message: "Rate Limit Exceeded"},
			expected: false,
		},
		"500 internal": {
			err:      &googleapi.Error{Code: 500, Message: "Internal Error"},
			expected: false,
		},
		"412 precondition": {
			err:      &googleapi.Error{Code: 412, Message: "Precondition Failed"},
			expected: false,
		},
		"wrapped 403": {
			err:      fmt.Errorf("deleting object: %w", &googleapi.Error{Code: 403, Message: "Forbidden"}),
			expected: false,
		},
		"errors.Join with 403": {
			err:      errors.Join(errors.New("network blip"), &googleapi.Error{Code: 403, Message: "Forbidden"}),
			expected: false,
		},
		"non-googleapi error": {
			err:      errors.New("network timeout"),
			expected: false,
		},
		// Typed nil in an error interface: err != nil is true, but there is no real
		// googleapi.Error value. Must not panic and must not be treated as ignorable.
		"typed nil *googleapi.Error": {
			err: func() error {
				var gerr *googleapi.Error
				return gerr
			}(),
			expected: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isIgnorableStorageObjectDeleteError(tc.err); got != tc.expected {
				t.Fatalf("isIgnorableStorageObjectDeleteError(%v) = %v, want %v", tc.err, got, tc.expected)
			}
		})
	}
}

// TestForceDestroyDeleteWorkerContract mirrors the force_destroy Objects.Delete
// worker: ignore 404/410, keep the first real error under concurrent deletes.
func TestForceDestroyDeleteWorkerContract(t *testing.T) {
	run := func(errs []error) error {
		var (
			mu  sync.Mutex
			got error
			wg  sync.WaitGroup
		)
		for _, e := range errs {
			e := e
			wg.Add(1)
			go func() {
				defer wg.Done()
				if e != nil {
					if isIgnorableStorageObjectDeleteError(e) {
						return
					}
					mu.Lock()
					defer mu.Unlock()
					if got == nil {
						got = e
					}
				}
			}()
		}
		wg.Wait()
		return got
	}

	t.Run("all success", func(t *testing.T) {
		if err := run([]error{nil, nil, nil}); err != nil {
			t.Fatalf("want nil, got %v", err)
		}
	})
	t.Run("all ignorable", func(t *testing.T) {
		if err := run([]error{
			&googleapi.Error{Code: 404},
			&googleapi.Error{Code: 404},
			&googleapi.Error{Code: 410},
		}); err != nil {
			t.Fatalf("want nil when only ignorable errors, got %v", err)
		}
	})
	t.Run("mix 404 and 403 keeps 403", func(t *testing.T) {
		err403 := &googleapi.Error{Code: 403, Message: "Forbidden"}
		err := run([]error{
			&googleapi.Error{Code: 404},
			err403,
			&googleapi.Error{Code: 410},
			nil,
		})
		if err == nil {
			t.Fatal("want 403 kept")
		}
		var gerr *googleapi.Error
		if !errors.As(err, &gerr) || gerr.Code != 403 {
			t.Fatalf("want 403, got %v", err)
		}
	})
	t.Run("contention keeps a non-ignorable error", func(t *testing.T) {
		var inputs []error
		for i := 0; i < 200; i++ {
			inputs = append(inputs, &googleapi.Error{Code: 500, Message: fmt.Sprintf("e%d", i)})
		}
		for i := 0; i < 200; i++ {
			inputs = append(inputs, &googleapi.Error{Code: 404})
		}
		err := run(inputs)
		if err == nil {
			t.Fatal("expected a kept error")
		}
		if isIgnorableStorageObjectDeleteError(err) {
			t.Fatalf("kept an ignorable error: %v", err)
		}
	})
}

func TestStorageBucketUpdate_ClientSideFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		priorAttrs     map[string]string
		diffAttrs      map[string]*terraform.ResourceAttrDiff
		wantPatchCalls int
		wantState      map[string]string
	}{
		{
			name: "force_destroy false to true short-circuits without PATCH and persists state",
			priorAttrs: map[string]string{
				"id":            "my-bucket",
				"project":       "my-proj",
				"name":          "my-bucket",
				"location":      "US",
				"force_destroy": "false",
			},
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"force_destroy": {Old: "false", New: "true"},
			},
			wantPatchCalls: 0,
			wantState: map[string]string{
				"force_destroy":   "true",
				"deletion_policy": "DELETE",
			},
		},
		{
			name: "force_destroy true to false short-circuits without PATCH and persists state",
			priorAttrs: map[string]string{
				"id":              "my-bucket",
				"project":         "my-proj",
				"name":            "my-bucket",
				"location":        "US",
				"force_destroy":   "true",
				"deletion_policy": "DELETE",
			},
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"force_destroy": {Old: "true", New: "false"},
			},
			wantPatchCalls: 0,
			wantState: map[string]string{
				"force_destroy":   "false",
				"deletion_policy": "DELETE",
			},
		},
		{
			name: "combined force_destroy and deletion_policy short-circuits without PATCH",
			priorAttrs: map[string]string{
				"id":              "my-bucket",
				"project":         "my-proj",
				"name":            "my-bucket",
				"location":        "US",
				"force_destroy":   "false",
				"deletion_policy": "DELETE",
			},
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"force_destroy":   {Old: "false", New: "true"},
				"deletion_policy": {Old: "DELETE", New: "ABANDON"},
			},
			wantPatchCalls: 0,
			wantState: map[string]string{
				"force_destroy":   "true",
				"deletion_policy": "ABANDON",
			},
		},
		{
			name: "force_destroy plus real mutable field requester_pays sends PATCH and persists both",
			priorAttrs: map[string]string{
				"id":              "my-bucket",
				"project":         "my-proj",
				"name":            "my-bucket",
				"location":        "US",
				"force_destroy":   "false",
				"requester_pays":  "false",
				"deletion_policy": "DELETE",
			},
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"force_destroy":  {Old: "false", New: "true"},
				"requester_pays": {Old: "false", New: "true"},
			},
			wantPatchCalls: 1,
			wantState: map[string]string{
				"force_destroy":  "true",
				"requester_pays": "true",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var patchCalls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodPatch:
					patchCalls++
					_ = json.NewEncoder(w).Encode(&raw.Bucket{
						Id:       "my-bucket",
						Name:     "my-bucket",
						Location: "US",
						SelfLink: "https://www.googleapis.com/storage/v1/b/my-bucket",
						Billing:  &raw.BucketBilling{RequesterPays: true},
					})
				case http.MethodGet:
					_ = json.NewEncoder(w).Encode(&raw.Bucket{
						Id:       "my-bucket",
						Name:     "my-bucket",
						Location: "US",
						SelfLink: "https://www.googleapis.com/storage/v1/b/my-bucket",
					})
				default:
					t.Errorf("unexpected HTTP method %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected method", http.StatusBadRequest)
				}
			}))
			defer server.Close()

			cfg := &transport_tpg.Config{
				Context:      context.Background(),
				Client:       server.Client(),
				Project:      "my-proj",
				PollInterval: time.Millisecond,
				CustomEndpoints: map[string]string{
					"storage_custom_endpoint": server.URL + "/storage/v1/",
				},
			}

			res := ResourceStorageBucket()
			prior := &terraform.InstanceState{
				ID:         "my-bucket",
				Attributes: tc.priorAttrs,
			}
			diff := &terraform.InstanceDiff{
				Attributes: tc.diffAttrs,
			}

			newState, diags := res.Apply(context.Background(), prior, diff, cfg)
			if diags.HasError() {
				t.Fatalf("Apply returned error diagnostics: %+v", diags)
			}
			if patchCalls != tc.wantPatchCalls {
				t.Fatalf("PATCH calls = %d, want %d", patchCalls, tc.wantPatchCalls)
			}
			for k, want := range tc.wantState {
				if got := newState.Attributes[k]; got != want {
					t.Errorf("newState.Attributes[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}
