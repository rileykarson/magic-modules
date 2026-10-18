package transport

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"testing"
)

// nonGDUUniverse is any universe domain other than googleapis.com.
// GetCurrentUserEmail returns early for those, which keeps these tests
// hermetic: logGoogleIdentities runs end to end without a userinfo request.
const nonGDUUniverse = "example.com"

// TestLogGoogleIdentities_logsTheIdentity pins down the fact that a plan or
// apply tells the user which identity the provider is running as, whenever
// anything is listening at INFO.
//
// This is load bearing for performance work on this function: the userinfo
// round trip it performs is the single most expensive thing in provider
// configuration, and it is tempting to move it off the critical path. Moving
// it to a goroutine was measured to drop this line on every single run,
// because Terraform tears the plugin process down as soon as the operation
// finishes, so any such change has to keep this test passing.
func TestLogGoogleIdentities_logsTheIdentity(t *testing.T) {
	// Each of these is, on its own, enough to make provider [INFO] output
	// reach somebody. See log_visibility.go.
	listeners := []struct {
		name string
		env  map[string]string
	}{
		{"TF_LOG=trace", map[string]string{"TF_LOG": "trace"}},
		{"TF_LOG=debug", map[string]string{"TF_LOG": "debug"}},
		{"TF_LOG=info", map[string]string{"TF_LOG": "info"}},
		{"TF_LOG=json", map[string]string{"TF_LOG": "json"}},
		{"TF_LOG_PROVIDER=trace", map[string]string{"TF_LOG_PROVIDER": "trace"}},
		{"TF_LOG_PROVIDER=info", map[string]string{"TF_LOG_PROVIDER": "info"}},
		{"TF_LOG_PROVIDER=info,TF_LOG=warn", map[string]string{"TF_LOG_PROVIDER": "info", "TF_LOG": "warn"}},
		{"TF_ACC=1", map[string]string{"TF_ACC": "1"}},
		{"TF_ACC_LOG_PATH", map[string]string{"TF_ACC_LOG_PATH": "/dev/null"}},
		{"TF_TEMP_LOG_PATH", map[string]string{"TF_TEMP_LOG_PATH": "/dev/null"}},
		{"override", map[string]string{"TF_GOOGLE_ALWAYS_LOG_IDENTITY": "1"}},
	}

	for _, l := range listeners {
		t.Run(l.name, func(t *testing.T) {
			setLogEnv(t, l.env)
			logs := captureLogs(t)

			ctx := context.Background()
			c := &Config{
				Context:        ctx,
				UserAgent:      "terraform-provider-google-beta/test",
				UniverseDomain: nonGDUUniverse,
				AccessToken:    "fake-access-token",
			}

			if err := c.logGoogleIdentities(ctx); err != nil {
				t.Fatalf("logGoogleIdentities returned an error: %s", err)
			}

			if got := logs.String(); !strings.Contains(got, "[INFO] Terraform is using this identity:") {
				t.Errorf("identity log line missing, logs were:\n%s", got)
			}
		})
	}
}

// TestLogGoogleIdentities_skipsUserinfoWhenNobodyIsListening is the other half
// of the contract. With no logging configured, Terraform discards provider
// [INFO] output on the client side, so producing it costs a blocking HTTPS
// round trip and buys nothing.
func TestLogGoogleIdentities_skipsUserinfoWhenNobodyIsListening(t *testing.T) {
	quiet := []struct {
		name string
		env  map[string]string
	}{
		{"nothing set", nil},
		{"TF_LOG=warn", map[string]string{"TF_LOG": "warn"}},
		{"TF_LOG=error", map[string]string{"TF_LOG": "error"}},
		{"TF_LOG_PROVIDER=off", map[string]string{"TF_LOG_PROVIDER": "off"}},
		{"TF_LOG_PROVIDER=error,TF_LOG=error", map[string]string{"TF_LOG_PROVIDER": "error", "TF_LOG": "error"}},
	}

	for _, q := range quiet {
		t.Run(q.name, func(t *testing.T) {
			setLogEnv(t, q.env)
			logs := captureLogs(t)

			ctx := context.Background()
			c := &Config{
				Context:        ctx,
				UserAgent:      "terraform-provider-google-beta/test",
				UniverseDomain: nonGDUUniverse,
				AccessToken:    "fake-access-token",
			}

			if err := c.logGoogleIdentities(ctx); err != nil {
				t.Fatalf("logGoogleIdentities returned an error: %s", err)
			}

			if got := logs.String(); strings.Contains(got, "Terraform is using this identity:") {
				t.Errorf("identity log line present although nothing is listening, logs were:\n%s", got)
			}
		})
	}
}

// TestLogGoogleIdentities_returnsTokenSourceError checks that a credential
// problem is surfaced to LoadAndValidate rather than only logged. This must
// hold whether or not logging is enabled: the token source is fetched for its
// own sake, not to produce a log line.
func TestLogGoogleIdentities_returnsTokenSourceError(t *testing.T) {
	for _, env := range []map[string]string{nil, {"TF_LOG": "trace"}} {
		setLogEnv(t, env)
		captureLogs(t)

		ctx := context.Background()
		c := &Config{
			Context:     ctx,
			UserAgent:   "terraform-provider-google-beta/test",
			Credentials: "this is not a credentials document",
		}

		if err := c.logGoogleIdentities(ctx); err == nil {
			t.Fatalf("logGoogleIdentities accepted invalid credentials with env %v, want an error", env)
		}
	}
}

// TestLogGoogleIdentities_impersonationReturnsSecondTokenSourceError covers
// the impersonation branch, which calls getTokenSource twice: once to recover
// the original identity for logging and once to restore the impersonated
// client. Only the second call is essential, and it must still run, and still
// report its error, when logging is disabled.
func TestLogGoogleIdentities_impersonationReturnsSecondTokenSourceError(t *testing.T) {
	for _, env := range []map[string]string{nil, {"TF_LOG": "trace"}} {
		setLogEnv(t, env)
		captureLogs(t)

		ctx := context.Background()
		c := &Config{
			Context:                   ctx,
			UserAgent:                 "terraform-provider-google-beta/test",
			UniverseDomain:            nonGDUUniverse,
			Credentials:               "this is not a credentials document",
			ImpersonateServiceAccount: "sa@example.iam.gserviceaccount.com",
		}

		if err := c.logGoogleIdentities(ctx); err == nil {
			t.Fatalf("impersonation branch accepted invalid credentials with env %v, want an error", env)
		}
	}
}

// setLogEnv puts the process into a known logging configuration and clears the
// memoized answer in InfoLogsVisible so the new one takes effect.
func setLogEnv(t *testing.T, env map[string]string) {
	t.Helper()

	for _, k := range []string{
		envTFLog, envTFLogProvider, envTFTempLogPath,
		envTFAcc, envTFAccLogPath, envAlwaysLogIdentity,
	} {
		t.Setenv(k, "")
		// t.Setenv cannot unset, and an empty value is what the predicate
		// treats as absent, so this is equivalent for our purposes.
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	resetInfoLogsVisible()
	t.Cleanup(resetInfoLogsVisible)
}

func resetInfoLogsVisible() {
	infoLogsVisibleOnce = sync.Once{}
	infoLogsVisibleCached = false
}

// captureLogs redirects the standard logger for the duration of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	buf := &bytes.Buffer{}
	previous := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(previous) })

	return buf
}
