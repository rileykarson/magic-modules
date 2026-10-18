package transport

import (
	"testing"
)

// TestParseTerraformLogLevel pins the mapping against
// terraform/internal/logging.parseLogLevel. The two surprising cases are
// "JSON", which is a real value users pass to TF_LOG and which means TRACE,
// and anything unrecognised, which means TRACE rather than silence.
func TestParseTerraformLogLevel(t *testing.T) {
	cases := map[string]logLevel{
		"":            logLevelOff,
		"trace":       logLevelTrace,
		"TRACE":       logLevelTrace,
		"Trace":       logLevelTrace,
		"debug":       logLevelDebug,
		"info":        logLevelInfo,
		"warn":        logLevelWarn,
		"error":       logLevelError,
		"off":         logLevelOff,
		"OFF":         logLevelOff,
		"json":        logLevelTrace,
		"JSON":        logLevelTrace,
		"nonsense":    logLevelTrace,
		" trace":      logLevelTrace, // Terraform does not trim; falls back to TRACE anyway.
		"warning":     logLevelTrace, // not a Terraform level
		"fatal":       logLevelTrace,
		"0":           logLevelTrace,
		"information": logLevelTrace,
	}

	for in, want := range cases {
		if got := parseTerraformLogLevel(in); got != want {
			t.Errorf("parseTerraformLogLevel(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestTerraformProviderLogLevel checks the precedence rule: TF_LOG_PROVIDER
// wins outright whenever it is non-empty, even when it disagrees with TF_LOG.
func TestTerraformProviderLogLevel(t *testing.T) {
	cases := []struct {
		tfLogProvider string
		tfLog         string
		want          logLevel
	}{
		{"", "", logLevelOff},
		{"", "trace", logLevelTrace},
		{"", "info", logLevelInfo},
		{"", "warn", logLevelWarn},
		{"", "json", logLevelTrace},
		{"trace", "", logLevelTrace},
		{"info", "", logLevelInfo},
		{"error", "", logLevelError},
		// Disagreement: the provider variable wins in both directions.
		{"error", "trace", logLevelError},
		{"trace", "error", logLevelTrace},
		{"off", "trace", logLevelOff},
		{"trace", "off", logLevelTrace},
		{"info", "warn", logLevelInfo},
		{"warn", "info", logLevelWarn},
		{"json", "warn", logLevelTrace},
	}

	for _, c := range cases {
		got := terraformProviderLogLevel(c.tfLogProvider, c.tfLog)
		if got != c.want {
			t.Errorf("terraformProviderLogLevel(TF_LOG_PROVIDER=%q, TF_LOG=%q) = %d, want %d",
				c.tfLogProvider, c.tfLog, got, c.want)
		}
	}
}

// TestSDKLogLevel pins the terraform-plugin-sdk mapping, which is not the same
// as Terraform's: it reads only TF_LOG and recognises neither "off" nor
// "json", so both mean TRACE.
func TestSDKLogLevel(t *testing.T) {
	cases := map[string]logLevel{
		"":      logLevelOff,
		"trace": logLevelTrace,
		"debug": logLevelDebug,
		"info":  logLevelInfo,
		"warn":  logLevelWarn,
		"error": logLevelError,
		"off":   logLevelTrace, // not an SDK level, so TRACE
		"json":  logLevelTrace, // not an SDK level, so TRACE
	}

	for in, want := range cases {
		if got := sdkLogLevel(in); got != want {
			t.Errorf("sdkLogLevel(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestInfoLogsVisible is the matrix that decides whether the userinfo round
// trip may be skipped. Any entry flipping from true to false is a user visible
// regression: a log line that used to appear stops appearing.
func TestInfoLogsVisible(t *testing.T) {
	cases := []struct {
		name string
		env  logVisibilityEnv
		want bool
	}{
		// The only case that matters for latency: a plain `terraform plan`
		// with no logging configured at all.
		{"nothing set", logVisibilityEnv{}, false},

		// TF_LOG alone.
		{"TF_LOG=trace", logVisibilityEnv{tfLog: "trace"}, true},
		{"TF_LOG=TRACE", logVisibilityEnv{tfLog: "TRACE"}, true},
		{"TF_LOG=debug", logVisibilityEnv{tfLog: "debug"}, true},
		{"TF_LOG=info", logVisibilityEnv{tfLog: "info"}, true},
		{"TF_LOG=json", logVisibilityEnv{tfLog: "json"}, true},
		{"TF_LOG=JSON", logVisibilityEnv{tfLog: "JSON"}, true},
		{"TF_LOG=warn", logVisibilityEnv{tfLog: "warn"}, false},
		{"TF_LOG=error", logVisibilityEnv{tfLog: "error"}, false},
		// Terraform silences its own logger for "off", but the SDK's mapping
		// does not recognise it, so we stay on the safe side and answer true.
		{"TF_LOG=off", logVisibilityEnv{tfLog: "off"}, true},
		// Unrecognised values mean TRACE in both mappings.
		{"TF_LOG=nonsense", logVisibilityEnv{tfLog: "nonsense"}, true},

		// TF_LOG_PROVIDER alone.
		{"TF_LOG_PROVIDER=trace", logVisibilityEnv{tfLogProvider: "trace"}, true},
		{"TF_LOG_PROVIDER=debug", logVisibilityEnv{tfLogProvider: "debug"}, true},
		{"TF_LOG_PROVIDER=info", logVisibilityEnv{tfLogProvider: "info"}, true},
		{"TF_LOG_PROVIDER=json", logVisibilityEnv{tfLogProvider: "json"}, true},
		{"TF_LOG_PROVIDER=warn", logVisibilityEnv{tfLogProvider: "warn"}, false},
		{"TF_LOG_PROVIDER=error", logVisibilityEnv{tfLogProvider: "error"}, false},
		{"TF_LOG_PROVIDER=off", logVisibilityEnv{tfLogProvider: "off"}, false},

		// Both set and disagreeing. TF_LOG_PROVIDER governs what Terraform
		// shows, but when TF_LOG is permissive the in-process SDK filter may
		// still be listening, so we answer true rather than risk it.
		{"provider=error log=trace", logVisibilityEnv{tfLogProvider: "error", tfLog: "trace"}, true},
		{"provider=off log=trace", logVisibilityEnv{tfLogProvider: "off", tfLog: "trace"}, true},
		{"provider=trace log=error", logVisibilityEnv{tfLogProvider: "trace", tfLog: "error"}, true},
		{"provider=info log=warn", logVisibilityEnv{tfLogProvider: "info", tfLog: "warn"}, true},
		{"provider=warn log=info", logVisibilityEnv{tfLogProvider: "warn", tfLog: "info"}, true},
		// Both quiet.
		{"provider=warn log=warn", logVisibilityEnv{tfLogProvider: "warn", tfLog: "warn"}, false},
		{"provider=error log=error", logVisibilityEnv{tfLogProvider: "error", tfLog: "error"}, false},
		{"provider=off log=error", logVisibilityEnv{tfLogProvider: "off", tfLog: "error"}, false},

		// Other listeners that bypass the level entirely.
		{"TF_TEMP_LOG_PATH", logVisibilityEnv{tfTempLogPath: "/tmp/x"}, true},
		{"TF_ACC", logVisibilityEnv{tfAcc: "1"}, true},
		{"TF_ACC_LOG_PATH", logVisibilityEnv{tfAccLogPath: "/tmp/x"}, true},
		{"override", logVisibilityEnv{alwaysLogIdentity: "1"}, true},
		// Overrides beat a quiet level.
		{"TF_ACC with TF_LOG=error", logVisibilityEnv{tfAcc: "1", tfLog: "error"}, true},
		{"override with TF_LOG_PROVIDER=off", logVisibilityEnv{alwaysLogIdentity: "1", tfLogProvider: "off"}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.env.infoLogsVisible(); got != c.want {
				t.Errorf("infoLogsVisible() = %v, want %v (env %+v)", got, c.want, c.env)
			}
		})
	}
}

// TestInfoLogsVisibleNeverQuieterThanTerraform is the safety property that
// actually matters, asserted over the whole cross product rather than over
// hand-picked rows: whenever Terraform's provider logger would emit an INFO
// line, the predicate must say so.
func TestInfoLogsVisibleNeverQuieterThanTerraform(t *testing.T) {
	values := []string{"", "trace", "TRACE", "debug", "info", "warn", "error", "off", "json", "JSON", "bogus"}

	for _, provider := range values {
		for _, global := range values {
			env := logVisibilityEnv{tfLogProvider: provider, tfLog: global}
			terraformWouldEmit := terraformProviderLogLevel(provider, global) <= logLevelInfo
			if terraformWouldEmit && !env.infoLogsVisible() {
				t.Errorf("TF_LOG_PROVIDER=%q TF_LOG=%q: Terraform would emit INFO but the predicate says no",
					provider, global)
			}
		}
	}
}

func TestTunePluginSDKLoggingForEnv(t *testing.T) {
	allTargets := append(append([]string{}, sdkLogEnvVars...), providerLogEnvVars...)

	expectAll := func(level string) map[string]string {
		m := make(map[string]string, len(allTargets))
		for _, k := range allTargets {
			m[k] = level
		}
		return m
	}

	cases := []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{
			name: "not a plugin process leaves env untouched",
			env:  map[string]string{},
			want: map[string]string{},
		},
		{
			name: "plugin process with no TF_LOG sets all SDK and provider loggers to OFF",
			env:  map[string]string{envPluginMagicCookie: "cookie"},
			want: expectAll("OFF"),
		},
		{
			name: "plugin process with TF_LOG=WARN sets all to WARN",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFLog:             "warn",
			},
			want: expectAll("WARN"),
		},
		{
			name: "plugin process with TF_LOG_PROVIDER=ERROR and TF_LOG=DEBUG sets all to ERROR",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFLogProvider:     "error",
				envTFLog:             "debug",
			},
			want: expectAll("ERROR"),
		},
		{
			name: "plugin process with TF_LOG=TRACE leaves env untouched",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFLog:             "trace",
			},
			want: map[string]string{},
		},
		{
			name: "plugin process with TF_LOG=JSON leaves env untouched",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFLog:             "json",
			},
			want: map[string]string{},
		},
		{
			name: "TF_ACC set bypasses tuning",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFAcc:             "1",
			},
			want: map[string]string{},
		},
		{
			name: "TF_TEMP_LOG_PATH set bypasses tuning",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFTempLogPath:     "/tmp/tf.log",
			},
			want: map[string]string{},
		},
		{
			name: "TF_LOG_PATH set bypasses tuning",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				envTFLogPath:         "/tmp/tf.log",
			},
			want: map[string]string{},
		},
		{
			name: "explicit TF_LOG_SDK preserves all SDK vars while setting provider vars",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				"TF_LOG_SDK":         "DEBUG",
			},
			want: map[string]string{
				"TF_LOG_PROVIDER_GOOGLE_BETA": "OFF",
				"TF_LOG_PROVIDER_GOOGLE":      "OFF",
			},
		},
		{
			name: "explicit sub-logger TF_LOG_SDK_PROTO is preserved while other SDK vars are set",
			env: map[string]string{
				envPluginMagicCookie: "cookie",
				"TF_LOG_SDK_PROTO":   "DEBUG",
			},
			want: map[string]string{
				"TF_LOG_SDK":                  "OFF",
				"TF_LOG_SDK_MUX":              "OFF",
				"TF_LOG_SDK_FRAMEWORK":        "OFF",
				"TF_LOG_SDK_HELPER_SCHEMA":    "OFF",
				"TF_LOG_SDK_HELPER_RESOURCE":  "OFF",
				"TF_LOG_PROVIDER_GOOGLE_BETA": "OFF",
				"TF_LOG_PROVIDER_GOOGLE":      "OFF",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]string{}
			lookup := func(k string) (string, bool) {
				if v, ok := got[k]; ok {
					return v, true
				}
				v, ok := tc.env[k]
				return v, ok
			}
			set := func(k, v string) {
				got[k] = v
			}

			tunePluginSDKLoggingForEnv(lookup, set)

			if len(got) != len(tc.want) {
				t.Fatalf("set %d vars (%v), want %d (%v)", len(got), got, len(tc.want), tc.want)
			}
			for k, wantVal := range tc.want {
				if got[k] != wantVal {
					t.Errorf("var %s = %q, want %q", k, got[k], wantVal)
				}
			}
		})
	}
}
