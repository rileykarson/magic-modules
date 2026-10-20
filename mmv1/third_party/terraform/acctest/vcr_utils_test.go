package acctest_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/dnaeon/go-vcr/cassette"
	"github.com/dnaeon/go-vcr/recorder"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func TestNewVcrMatcherFunc_canDetectMatches(t *testing.T) {

	// Everything should be determined as a match in this test
	cases := map[string]struct {
		httpRequest     requestDescription
		cassetteRequest requestDescription
	}{
		"matches POST requests with empty bodies": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{}",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{}",
			},
		},
		"matches POST requests with exact matching bodies": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field\":\"value\"}",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field\":\"value\"}",
			},
		},
		"matches GET requests when SendRequest adds alt=json but cassette has alt=json&prettyPrint=false": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance",
				query:  "alt=json",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance",
				query:  "alt=json&prettyPrint=false",
			},
		},
		"matches GET requests when both have alt and prettyPrint in different order": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance",
				query:  "prettyPrint=false&alt=json",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance",
				query:  "alt=json&prettyPrint=false",
			},
		},
		"matches POST requests with matching but re-ordered bodies, but only if Content-Type contains 'application/json'": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				headers: map[string]string{
					"Content-Type": "application/json",
				},
				body: "{\"field1\":\"value1\",\"field2\":\"value2\"}", // 1 before 2
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				headers: map[string]string{
					"Content-Type": "application/json",
				},
				body: "{\"field2\":\"value2\",\"field1\":\"value1\"}", // 2 before 1
			},
		},
	}

	for tn, tc := range cases {
		t.Run(tn, func(t *testing.T) {
			// Make matcher
			ctx := context.Background()
			req := prepareHttpRequest(tc.httpRequest)
			cassetteReq := prepareCassetteRequest(tc.cassetteRequest)
			matcher := acctest.NewVcrMatcherFunc(ctx)

			// Act - use matcher
			matchDetected := matcher(req, cassetteReq)

			// Assert match
			if !matchDetected {
				t.Fatalf("expected matcher to match the requests")
			}
		})
	}
}

func TestNewVcrMatcherFunc_canDetectMismatches(t *testing.T) {

	// All these cases are expected to end with no match detected
	cases := map[string]struct {
		httpRequest     requestDescription
		cassetteRequest requestDescription
	}{
		"requests using different schemes": {
			httpRequest: requestDescription{
				scheme: "http",
				method: "GET",
				host:   "example.com",
				path:   "foobar",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "example.com",
				path:   "foobar",
			},
		},
		"requests using different hosts": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "example.com",
				path:   "foobar",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "google.com",
				path:   "foobar",
			},
		},
		"requests using different paths": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "example.com",
				path:   "foobar1",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "example.com",
				path:   "foobar2",
			},
		},
		"requests with different methods": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{}",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "PUT",
				host:   "example.com",
				path:   "foobar",
				body:   "{}",
			},
		},
		"POST requests with different bodies": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field\":\"value is ABCDEFG\"}",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field\":\"value is MNLOP\"}",
			},
		},
		"requests with different non-standard query params do not match": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance/detachDisk",
				query:  "deviceName=disk-1&alt=json",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/v1/projects/my-project/zones/us-central1-a/instances/my-instance/detachDisk",
				query:  "deviceName=disk-2&alt=json&prettyPrint=false",
			},
		},
		"POST requests with matching but re-ordered bodies aren't matching if Content-Type header is not 'application/json'": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				headers: map[string]string{
					"Content-Type": "foobar",
				},
				body: "{\"field1\":\"value1\",\"field2\":\"value2\"}", // 1 before 2
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				headers: map[string]string{
					"Content-Type": "foobar",
				},
				body: "{\"field2\":\"value2\",\"field1\":\"value1\"}", // 2 before 1
			},
		},
	}

	for tn, tc := range cases {
		t.Run(tn, func(t *testing.T) {
			// Make matcher
			ctx := context.Background()
			req := prepareHttpRequest(tc.httpRequest)
			cassetteReq := prepareCassetteRequest(tc.cassetteRequest)
			matcher := acctest.NewVcrMatcherFunc(ctx)

			// Act - use matcher
			matchDetected := matcher(req, cassetteReq)

			// Assert match
			if matchDetected {
				t.Fatalf("expected matcher to not match the requests")
			}
		})
	}
}

// Currently there is no code to actively force the matcher to overlook differing User-Agent values.
// It isn't checked at any point in the matcher logic.
func TestNewVcrMatcherFunc_ignoresDifferentUserAgents(t *testing.T) {

	cases := map[string]struct {
		httpRequest     requestDescription
		cassetteRequest requestDescription
	}{
		"GET requests with different useragents are matched": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "example.com",
				path:   "foobar",
				headers: map[string]string{
					"User-Agent": "user-agent-HTTP",
				},
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "example.com",
				path:   "foobar",
				headers: map[string]string{
					"User-Agent": "user-agent-CASSETTE",
				},
			},
		},
		"POST requests with identical bodies and different useragents are matched": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field\":\"value\"}",
				headers: map[string]string{
					"User-Agent": "user-agent-HTTP",
				},
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field\":\"value\"}",
				headers: map[string]string{
					"User-Agent": "user-agent-CASSETTE",
				},
			},
		},
		"POST requests with reordered but matching bodies and different useragents are matched if Content-Type contains 'application/json'": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field1\":\"value1\",\"field2\":\"value2\"}",
				headers: map[string]string{
					"User-Agent":   "user-agent-HTTP",
					"Content-Type": "application/json",
				},
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "POST",
				host:   "example.com",
				path:   "foobar",
				body:   "{\"field2\":\"value2\",\"field1\":\"value1\"}",
				headers: map[string]string{
					"User-Agent":   "user-agent-CASSETTE",
					"Content-Type": "application/json",
				},
			},
		},
	}

	for tn, tc := range cases {
		t.Run(tn, func(t *testing.T) {
			// Make matcher
			ctx := context.Background()
			req := prepareHttpRequest(tc.httpRequest)
			cassetteReq := prepareCassetteRequest(tc.cassetteRequest)
			matcher := acctest.NewVcrMatcherFunc(ctx)

			// Act - use matcher
			matchDetected := matcher(req, cassetteReq)

			// Assert match
			if !matchDetected {
				t.Fatalf("expected matcher to match the requests")
			}
		})
	}
}

type requestDescription struct {
	scheme  string
	method  string
	host    string
	path    string
	body    string
	query   string
	headers map[string]string
}

func prepareHttpRequest(d requestDescription) *http.Request {
	url := &url.URL{
		Scheme:   d.scheme,
		Host:     d.host,
		Path:     d.path,
		RawQuery: d.query,
	}

	req := &http.Request{
		Method: d.method,
		URL:    url,
	}

	// Conditionally set a body
	if d.body != "" {
		body := io.NopCloser(bytes.NewBufferString(d.body))
		req.Body = body
	}
	// Conditionally set headers
	if len(d.headers) > 0 {
		req.Header = http.Header{}
		for k, v := range d.headers {
			req.Header.Set(k, v)
		}
	}

	return req
}

func prepareCassetteRequest(d requestDescription) cassette.Request {
	fullUrl := fmt.Sprintf("%s://%s/%s", d.scheme, d.host, d.path)
	if d.query != "" {
		fullUrl = fmt.Sprintf("%s?%s", fullUrl, d.query)
	}

	req := cassette.Request{
		Method: d.method,
		URL:    fullUrl,
	}

	// Conditionally set a body
	if d.body != "" {
		req.Body = d.body
	}
	// Conditionally set headers
	if len(d.headers) > 0 {
		req.Headers = http.Header{}
		for k, v := range d.headers {
			req.Headers.Add(k, v)
		}
	}

	return req
}

func TestIsVcrReplaying(t *testing.T) {
	cases := []struct {
		name     string
		vcrPath  string
		vcrMode  string
		expected bool
	}{
		{"empty", "", "", false},
		{"path only", "/tmp", "", false},
		{"mode recording", "/tmp", "RECORDING", false},
		{"mode replaying no path", "", "REPLAYING", false},
		{"mode replaying with path", "/tmp", "REPLAYING", true},
		{"mode other", "/tmp", "OTHER", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VCR_PATH", tc.vcrPath)
			t.Setenv("VCR_MODE", tc.vcrMode)
			if got := acctest.IsVcrReplaying(); got != tc.expected {
				t.Errorf("IsVcrReplaying() = %v, expected %v", got, tc.expected)
			}
		})
	}
}

func TestRunVcrSubtest(t *testing.T) {
	origPath := os.Getenv("VCR_PATH")
	origMode := os.Getenv("VCR_MODE")
	defer func() {
		os.Setenv("VCR_PATH", origPath)
		os.Setenv("VCR_MODE", origMode)
	}()

	os.Setenv("VCR_PATH", "")
	os.Setenv("VCR_MODE", "")
	acctest.RunVcrSubtest(t, "disabled when not replaying", func(t *testing.T) {
		// Calling t.Parallel() here should not panic since RunVcrSubtest didn't call it
		t.Parallel()
	})

	os.Setenv("VCR_PATH", "/tmp")
	os.Setenv("VCR_MODE", "REPLAYING")
	acctest.RunVcrSubtest(t, "enabled when replaying", func(t *testing.T) {
		// In Go testing, calling t.Parallel() a second time on the same testing.T panics
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("expected panic when calling t.Parallel() a second time, but did not panic")
			}
		}()
		t.Parallel()
	})
}

func TestBootstrapConfig_usesProviderConfiguration(t *testing.T) {
	t.Setenv("TF_ACC", "1")
	t.Setenv("GOOGLE_CREDENTIALS", "")
	t.Setenv("GOOGLE_CREDENTIALS_FILE", transport_tpg.TestFakeCredentialsPath)
	t.Setenv("GOOGLE_PROJECT", "my-bootstrap-project")
	t.Setenv("GOOGLE_REGION", "us-central1")
	t.Setenv("GOOGLE_ZONE", "us-central1-a")
	t.Setenv("GOOGLE_BILLING_PROJECT", "my-billing-project")
	t.Setenv("USER_PROJECT_OVERRIDE", "true")
	t.Setenv("CLOUDSDK_CORE_REQUEST_REASON", "bootstrap-reason")
	t.Setenv("GOOGLE_TERRAFORM_USERAGENT_EXTENSION", "bootstrap-ua-ext")
	t.Setenv("GOOGLE_COMPUTE_CUSTOM_ENDPOINT", "https://custom.compute.googleapis.com/compute/v1/")

	config := transport_tpg.BootstrapConfig(t)
	if config == nil {
		t.Fatalf("expected non-nil config from BootstrapConfig")
	}

	if config.Project != "my-bootstrap-project" {
		t.Errorf("expected Project %q, got %q", "my-bootstrap-project", config.Project)
	}
	if config.Region != "us-central1" {
		t.Errorf("expected Region %q, got %q", "us-central1", config.Region)
	}
	if config.Zone != "us-central1-a" {
		t.Errorf("expected Zone %q, got %q", "us-central1-a", config.Zone)
	}
	if config.BillingProject != "my-billing-project" {
		t.Errorf("expected BillingProject %q, got %q", "my-billing-project", config.BillingProject)
	}
	if !config.UserProjectOverride {
		t.Errorf("expected UserProjectOverride to be true")
	}
	if config.RequestReason != "bootstrap-reason" {
		t.Errorf("expected RequestReason %q, got %q", "bootstrap-reason", config.RequestReason)
	}
	if !strings.Contains(config.UserAgent, "terraform-provider-google") || !strings.Contains(config.UserAgent, "bootstrap-ua-ext") {
		t.Errorf("expected UserAgent to contain provider prefix and extension, got %q", config.UserAgent)
	}
	if got := config.CustomEndpoints["compute_custom_endpoint"]; got != "https://custom.compute.googleapis.com/compute/v1/" {
		t.Errorf("expected compute_custom_endpoint %q, got %q", "https://custom.compute.googleapis.com/compute/v1/", got)
	}
	if config.BatchingConfig == nil || config.RequestBatcherServiceUsage == nil || config.RequestBatcherIam == nil {
		t.Errorf("expected non-nil BatchingConfig and request batchers")
	}
}

func TestBootstrapConfig_vcrIsolation(t *testing.T) {
	vcrDir := t.TempDir()
	t.Setenv("TF_ACC", "1")
	t.Setenv("GOOGLE_CREDENTIALS", transport_tpg.TestFakeCredentialsPath)
	t.Setenv("GOOGLE_PROJECT", "my-vcr-project")
	t.Setenv("GOOGLE_REGION", "us-central1")
	t.Setenv("GOOGLE_ZONE", "us-central1-a")
	t.Setenv("VCR_MODE", "RECORDING")
	t.Setenv("VCR_PATH", vcrDir)

	// 1. BootstrapConfig called before VCR provider configuration must NOT attach a VCR recorder
	// and must NOT populate acctest's VCR configs cache.
	bootstrapCfg := transport_tpg.BootstrapConfig(t)
	if bootstrapCfg == nil || bootstrapCfg.Client == nil {
		t.Fatalf("expected non-nil bootstrap config and HTTP client")
	}
	if _, ok := bootstrapCfg.Client.Transport.(*recorder.Recorder); ok {
		t.Fatalf("expected BootstrapConfig transport NOT to be a *recorder.Recorder")
	}

	// 2. Configuring the VCR SDK provider for this test must still attach a *recorder.Recorder
	// (proving BootstrapConfig did not pollute configs[t.Name()]).
	sdkProv := acctest.GetSDKProvider(t.Name())
	diags := sdkProv.Configure(context.Background(), &terraform.ResourceConfig{})
	if diags.HasError() {
		t.Fatalf("unexpected error configuring SDK provider: %v", diags)
	}
	vcrCfg := acctest.GoogleProviderConfig(t)
	rec, ok := vcrCfg.Client.Transport.(*recorder.Recorder)
	if !ok {
		t.Fatalf("expected VCR provider transport to be *recorder.Recorder, got %T", vcrCfg.Client.Transport)
	}
	defer func() {
		_ = rec.Stop()
	}()

	// 3. Calling BootstrapConfig again while VCR config is cached for t.Name() must STILL
	// return a non-VCR config (not the cached VCR config).
	bootstrapCfgDuringVcr := transport_tpg.BootstrapConfig(t)
	if _, ok := bootstrapCfgDuringVcr.Client.Transport.(*recorder.Recorder); ok {
		t.Fatalf("expected BootstrapConfig called during VCR test NOT to use *recorder.Recorder")
	}
}
