package acctest_test

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
		"matches requests with identical X-Goog-Api-Version headers": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
				headers: map[string]string{
					"X-Goog-Api-Version": "2026-11-01",
				},
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
				headers: map[string]string{
					"X-Goog-Api-Version": "2026-11-01",
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
		"requests where HTTP request has X-Goog-Api-Version header and cassette does not": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
				headers: map[string]string{
					"X-Goog-Api-Version": "2026-11-01",
				},
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
			},
		},
		"requests where cassette has X-Goog-Api-Version header and HTTP request does not": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
				headers: map[string]string{
					"X-Goog-Api-Version": "2026-11-01",
				},
			},
		},
		"requests with different X-Goog-Api-Version headers": {
			httpRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
				headers: map[string]string{
					"X-Goog-Api-Version": "2026-11-01",
				},
			},
			cassetteRequest: requestDescription{
				scheme: "https",
				method: "GET",
				host:   "compute.googleapis.com",
				path:   "compute/beta/projects/my-project/global/urlMaps/my-url-map",
				headers: map[string]string{
					"X-Goog-Api-Version": "2026-05-01",
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
	t.Run("provider_fields_and_credentials_file_without_setenv", func(t *testing.T) {
		t.Setenv("GOOGLE_CREDENTIALS_FILE", transport_tpg.TestFakeCredentialsPath)
		acctest.UnsetTestProviderConfigEnvs(t)
		if got := os.Getenv("GOOGLE_CREDENTIALS_FILE"); got != "" {
			t.Fatalf("expected UnsetTestProviderConfigEnvs to unset GOOGLE_CREDENTIALS_FILE, got %q", got)
		}

		t.Setenv("TF_LOG", "")
		t.Setenv("TF_ACC", "1")
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
		if envCreds := os.Getenv("GOOGLE_CREDENTIALS"); envCreds != "" {
			t.Fatalf("expected BootstrapConfig not to mutate GOOGLE_CREDENTIALS env var, got %q", envCreds)
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
	})

	t.Run("credentials_precedence_and_universe_domain", func(t *testing.T) {
		acctest.UnsetTestProviderConfigEnvs(t)
		t.Setenv("TF_LOG", "")
		t.Setenv("TF_ACC", "1")
		t.Setenv("GOOGLE_PROJECT", "my-precedence-project")
		t.Setenv("GOOGLE_REGION", "us-central1")
		t.Setenv("GOOGLE_ZONE", "us-central1-a")

		// 1. Explicit GOOGLE_CREDENTIALS wins over GOOGLE_CREDENTIALS_FILE.
		explicitCreds := acctest.GenerateFakeCredentialsJson("explicit-creds")
		t.Setenv("GOOGLE_CREDENTIALS", explicitCreds)
		t.Setenv("GOOGLE_CREDENTIALS_FILE", "/nonexistent/creds.json")
		cfg := transport_tpg.BootstrapConfig(t)
		if cfg == nil || cfg.Credentials != explicitCreds {
			t.Fatalf("expected explicit GOOGLE_CREDENTIALS to win over GOOGLE_CREDENTIALS_FILE, got %+v", cfg)
		}

		// 2. GOOGLE_OAUTH_ACCESS_TOKEN wins over GOOGLE_CREDENTIALS_FILE.
		t.Setenv("GOOGLE_CREDENTIALS", "")
		t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "fake-oauth-token")
		cfg = transport_tpg.BootstrapConfig(t)
		if cfg == nil || cfg.AccessToken != "fake-oauth-token" || cfg.Credentials != "" {
			t.Fatalf("expected GOOGLE_OAUTH_ACCESS_TOKEN to win over GOOGLE_CREDENTIALS_FILE, got %+v", cfg)
		}

		// 3. GOOGLE_USE_DEFAULT_CREDENTIALS=true uses ADC without reading GOOGLE_CREDENTIALS_FILE.
		t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "")
		t.Setenv("GOOGLE_USE_DEFAULT_CREDENTIALS", "true")
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", transport_tpg.TestFakeCredentialsPath)
		cfg = transport_tpg.BootstrapConfig(t)
		if cfg == nil || cfg.Credentials != "" {
			t.Fatalf("expected GOOGLE_USE_DEFAULT_CREDENTIALS=true to leave Credentials empty and use ADC, got %+v", cfg)
		}

		// 4. GOOGLE_CREDENTIALS_FILE and GOOGLE_UNIVERSE_DOMAIN work together without overwriting rc.Config.
		acctest.UnsetTestProviderConfigEnvs(t)
		t.Setenv("TF_LOG", "")
		t.Setenv("GOOGLE_PROJECT", "my-precedence-project")
		t.Setenv("GOOGLE_REGION", "us-central1")
		t.Setenv("GOOGLE_ZONE", "us-central1-a")
		rsaKey, err := rsa.GenerateKey(crand.Reader, 2048)
		if err != nil {
			t.Fatalf("failed generating test RSA key: %v", err)
		}
		pemBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
		})
		udCredsBytes, err := json.Marshal(map[string]string{
			"type":            "service_account",
			"project_id":      "my-precedence-project",
			"private_key_id":  "foo",
			"private_key":     string(pemBytes),
			"client_email":    "ud@example.com",
			"client_id":       "1234567890",
			"universe_domain": "apis-tpclp.goog",
		})
		if err != nil {
			t.Fatalf("failed marshaling universe_domain creds: %v", err)
		}
		udCredsFile := filepath.Join(t.TempDir(), "ud_creds.json")
		if err := os.WriteFile(udCredsFile, udCredsBytes, 0600); err != nil {
			t.Fatalf("failed writing temp universe_domain creds: %v", err)
		}
		t.Setenv("GOOGLE_CREDENTIALS_FILE", udCredsFile)
		t.Setenv("GOOGLE_UNIVERSE_DOMAIN", "apis-tpclp.goog")
		cfg = transport_tpg.BootstrapConfig(t)
		if cfg == nil || cfg.UniverseDomain != "apis-tpclp.goog" {
			t.Fatalf("expected UniverseDomain %q with GOOGLE_CREDENTIALS_FILE, got %+v", "apis-tpclp.goog", cfg)
		}
	})
}

func TestBootstrapConfig_vcrIsolation(t *testing.T) {
	t.Run("recording_mode_and_watch_requests", func(t *testing.T) {
		t.Cleanup(func() { acctest.CleanupVcrConfigForTest(t) })

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		vcrDir := t.TempDir()
		acctest.UnsetTestProviderConfigEnvs(t)
		t.Setenv("TF_LOG", "")
		t.Setenv("TF_ACC", "1")
		t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "fake-test-token")
		t.Setenv("GOOGLE_PROJECT", "my-vcr-project")
		t.Setenv("GOOGLE_REGION", "us-central1")
		t.Setenv("GOOGLE_ZONE", "us-central1-a")
		t.Setenv("VCR_MODE", "RECORDING")
		t.Setenv("VCR_PATH", vcrDir)

		var watchedCount atomic.Int32
		acctest.WatchRequests(t, func(req *http.Request) {
			watchedCount.Add(1)
		})

		// 1. BootstrapConfig before VCR setup: neither attaches *recorder.Recorder, nor populates
		// configs[t.Name()], nor triggers WatchRequests.
		bootstrapBefore := transport_tpg.BootstrapConfig(t)
		if bootstrapBefore == nil || bootstrapBefore.Client == nil {
			t.Fatalf("expected non-nil bootstrap config and HTTP client")
		}
		if _, ok := bootstrapBefore.Client.Transport.(*recorder.Recorder); ok {
			t.Fatalf("expected BootstrapConfig transport NOT to be *recorder.Recorder")
		}
		resp, err := bootstrapBefore.Client.Get(srv.URL + "/bootstrap-before")
		if err != nil {
			t.Fatalf("unexpected error on bootstrapBefore request: %v", err)
		}
		_ = resp.Body.Close()
		if got := watchedCount.Load(); got != 0 {
			t.Fatalf("expected 0 watched requests after BootstrapConfig, got %d", got)
		}

		// 2. GoogleProviderConfig on a cache miss (before GetSDKProvider.Configure) DOES wrap with WatchRequests.
		cacheMissProviderCfg := acctest.GoogleProviderConfig(t)
		resp, err = cacheMissProviderCfg.Client.Get(srv.URL + "/provider-cache-miss")
		if err != nil {
			t.Fatalf("unexpected error on cache-miss GoogleProviderConfig request: %v", err)
		}
		_ = resp.Body.Close()
		if got := watchedCount.Load(); got != 1 {
			t.Fatalf("expected 1 watched request after cache-miss GoogleProviderConfig, got %d", got)
		}

		// 3. Configure VCR SDK provider: still attaches *recorder.Recorder (configs[t.Name()] was not polluted).
		sdkProv := acctest.GetSDKProvider(t.Name())
		diags := sdkProv.Configure(context.Background(), &terraform.ResourceConfig{})
		if diags.HasError() {
			t.Fatalf("unexpected error configuring SDK provider: %v", diags)
		}
		vcrCfg := acctest.GoogleProviderConfig(t)
		resp, err = vcrCfg.Client.Get(srv.URL + "/provider-recorded")
		if err != nil {
			t.Fatalf("unexpected error on VCR provider request: %v", err)
		}
		_ = resp.Body.Close()
		if got := watchedCount.Load(); got != 2 {
			t.Fatalf("expected 2 watched requests after VCR GoogleProviderConfig, got %d", got)
		}

		// 4. BootstrapConfig during active VCR test: still isolated from VCR cassette and WatchRequests.
		bootstrapDuring := transport_tpg.BootstrapConfig(t)
		resp, err = bootstrapDuring.Client.Get(srv.URL + "/bootstrap-during")
		if err != nil {
			t.Fatalf("unexpected error on bootstrapDuring request: %v", err)
		}
		_ = resp.Body.Close()
		if got := watchedCount.Load(); got != 2 {
			t.Fatalf("expected watched count to remain 2 after bootstrapDuring, got %d", got)
		}

		// 5. Flush VCR recorder via CleanupVcrConfigForTest and verify cassette recorded ONLY /provider-recorded.
		acctest.CleanupVcrConfigForTest(t)
		cassetteName := filepath.Join(vcrDir, strings.ReplaceAll(t.Name(), "/", "_"))
		recorded, err := cassette.Load(cassetteName)
		if err != nil {
			t.Fatalf("failed loading saved cassette %q: %v", cassetteName, err)
		}
		if len(recorded.Interactions) != 1 {
			t.Fatalf("expected exactly 1 recorded interaction in cassette, got %d", len(recorded.Interactions))
		}
		if !strings.HasSuffix(recorded.Interactions[0].Request.URL, "/provider-recorded") {
			t.Fatalf("expected recorded URL to end with /provider-recorded, got %q", recorded.Interactions[0].Request.URL)
		}
	})

	t.Run("replaying_mode_isolation", func(t *testing.T) {
		t.Cleanup(func() { acctest.CleanupVcrConfigForTest(t) })

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"live":true}`))
		}))
		defer srv.Close()

		vcrDir := t.TempDir()
		// Write an empty cassette YAML directly on disk (cassette.Save() is a no-op when
		// Interactions is empty, and recorder.NewAsMode falls back to ModeRecording if the
		// .yaml file does not exist).
		cassetteName := filepath.Join(vcrDir, strings.ReplaceAll(t.Name(), "/", "_"))
		if err := os.WriteFile(cassetteName+".yaml", []byte("---\nversion: 1\ninteractions: []\n"), 0600); err != nil {
			t.Fatalf("failed writing empty cassette file: %v", err)
		}

		acctest.UnsetTestProviderConfigEnvs(t)
		t.Setenv("TF_LOG", "")
		t.Setenv("TF_ACC", "1")
		t.Setenv("GOOGLE_OAUTH_ACCESS_TOKEN", "fake-test-token")
		t.Setenv("GOOGLE_PROJECT", "my-vcr-replay-project")
		t.Setenv("GOOGLE_REGION", "us-central1")
		t.Setenv("GOOGLE_ZONE", "us-central1-a")
		t.Setenv("VCR_MODE", "REPLAYING")
		t.Setenv("VCR_PATH", vcrDir)

		sdkProv := acctest.GetSDKProvider(t.Name())
		diags := sdkProv.Configure(context.Background(), &terraform.ResourceConfig{})
		if diags.HasError() {
			t.Fatalf("unexpected error configuring SDK provider in REPLAYING: %v", diags)
		}

		vcrCfg := acctest.GoogleProviderConfig(t)
		if vcrCfg.PollInterval != time.Millisecond || vcrCfg.SleepUnit != time.Millisecond {
			t.Fatalf("expected VCR replay PollInterval/SleepUnit to be 1ms, got %v / %v", vcrCfg.PollInterval, vcrCfg.SleepUnit)
		}
		if _, err := vcrCfg.Client.Get(srv.URL + "/unrecorded"); err == nil {
			t.Fatalf("expected unrecorded request via VCR replay client to fail, but succeeded")
		}

		bootstrapCfg := transport_tpg.BootstrapConfig(t)
		if bootstrapCfg.PollInterval != 10*time.Second || bootstrapCfg.SleepUnit != time.Second {
			t.Fatalf("expected live PollInterval=10s and SleepUnit=1s on bootstrap config, got %v / %v", bootstrapCfg.PollInterval, bootstrapCfg.SleepUnit)
		}
		if bootstrapCfg.RequestBatcherServiceUsage == vcrCfg.RequestBatcherServiceUsage || bootstrapCfg.RequestBatcherIam == vcrCfg.RequestBatcherIam {
			t.Fatalf("expected BootstrapConfig to have independent request batchers from VCR config")
		}
		resp, err := bootstrapCfg.Client.Get(srv.URL + "/live-bootstrap")
		if err != nil {
			t.Fatalf("expected BootstrapConfig client to make live HTTP call during REPLAYING, got error: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK from live bootstrap call, got %d", resp.StatusCode)
		}
	})
}

func TestBootstrapConfig_parallelRunVcrSubtest(t *testing.T) {
	vcrDir := t.TempDir()
	saved := make(map[string]*string)
	setEnv := func(k, v string) {
		if _, exists := saved[k]; !exists {
			if cur, ok := os.LookupEnv(k); ok {
				val := cur
				saved[k] = &val
			} else {
				saved[k] = nil
			}
		}
		os.Setenv(k, v)
	}
	t.Cleanup(func() {
		for k, v := range saved {
			if v == nil {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, *v)
			}
		}
	})

	setEnv("TF_LOG", "")
	setEnv("TF_ACC", "1")
	setEnv("GOOGLE_CREDENTIALS", "")
	setEnv("GOOGLE_CLOUD_KEYFILE_JSON", "")
	setEnv("GCLOUD_KEYFILE_JSON", "")
	setEnv("GOOGLE_OAUTH_ACCESS_TOKEN", "")
	setEnv("GOOGLE_USE_DEFAULT_CREDENTIALS", "")
	setEnv("GOOGLE_CREDENTIALS_FILE", transport_tpg.TestFakeCredentialsPath)
	setEnv("GOOGLE_PROJECT", "my-parallel-project")
	setEnv("GOOGLE_REGION", "us-central1")
	setEnv("GOOGLE_ZONE", "us-central1-a")
	setEnv("VCR_PATH", vcrDir)
	setEnv("VCR_MODE", "REPLAYING")

	t.Run("group", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			acctest.RunVcrSubtest(t, fmt.Sprintf("sub_%d", i), func(t *testing.T) {
				cfg := transport_tpg.BootstrapConfig(t)
				if cfg == nil || cfg.Project != "my-parallel-project" {
					t.Errorf("unexpected config in parallel subtest: %+v", cfg)
				}
			})
		}
	})
}
