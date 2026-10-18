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
