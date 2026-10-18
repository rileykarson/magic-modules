package transport

import (
	"os"
	"strings"
)

// logLevel mirrors hclog.Level. The numeric values matter: Terraform compares
// levels numerically, and "off" is the highest, not the lowest.
type logLevel int

const (
	logLevelNone  logLevel = 0 // hclog.NoLevel - emits everything
	logLevelTrace logLevel = 1
	logLevelDebug logLevel = 2
	logLevelInfo  logLevel = 3
	logLevelWarn  logLevel = 4
	logLevelError logLevel = 5
	logLevelOff   logLevel = 6
)

// Environment variables consulted. Names match Terraform and
// terraform-plugin-sdk rather than being invented here.
const (
	envTFLog = "TF_LOG"
	// TF_LOG_PROVIDER overrides TF_LOG for plugin processes.
	envTFLogProvider = "TF_LOG_PROVIDER"
	// TF_TEMP_LOG_PATH makes Terraform register an unconditional TRACE sink
	// on the named file (terraform/main.go), which bypasses the provider
	// logger's level entirely.
	envTFTempLogPath = "TF_TEMP_LOG_PATH"
	// Acceptance tests serve the provider in-process and filter with
	// terraform-plugin-sdk/v2/helper/logging, which has its own mapping.
	envTFAcc        = "TF_ACC"
	envTFAccLogPath = "TF_ACC_LOG_PATH"
	// TF_LOG_PATH is Terraform's log file output path; when set without
	// TF_LOG, leave SDK loggers untouched.
	envTFLogPath = "TF_LOG_PATH"
	// TF_PLUGIN_MAGIC_COOKIE is set by Terraform on plugin child processes.
	envPluginMagicCookie = "TF_PLUGIN_MAGIC_COOKIE"
)

var (
	sdkLogEnvVars = []string{
		"TF_LOG_SDK",
		"TF_LOG_SDK_PROTO",
		"TF_LOG_SDK_MUX",
		"TF_LOG_SDK_FRAMEWORK",
		"TF_LOG_SDK_HELPER_SCHEMA",
		"TF_LOG_SDK_HELPER_RESOURCE",
	}
	providerLogEnvVars = []string{
		"TF_LOG_PROVIDER_GOOGLE_BETA",
		"TF_LOG_PROVIDER_GOOGLE",
	}
)

func (l logLevel) String() string {
	switch l {
	case logLevelTrace, logLevelNone:
		return "TRACE"
	case logLevelDebug:
		return "DEBUG"
	case logLevelInfo:
		return "INFO"
	case logLevelWarn:
		return "WARN"
	case logLevelError:
		return "ERROR"
	case logLevelOff:
		return "OFF"
	default:
		return "TRACE"
	}
}

// parseTerraformLogLevel reproduces terraform/internal/logging.parseLogLevel.
//
// Terraform uppercases the raw value without trimming, maps the pseudo-level
// "JSON" to TRACE, and falls back to TRACE - not to off - for anything it does
// not recognise. "OFF" is a recognised level and does silence the logger.
func parseTerraformLogLevel(raw string) logLevel {
	level := strings.ToUpper(raw)
	if level == "" {
		return logLevelOff
	}
	if level == "JSON" {
		level = "TRACE"
	}

	switch level {
	case "TRACE":
		return logLevelTrace
	case "DEBUG":
		return logLevelDebug
	case "INFO":
		return logLevelInfo
	case "WARN":
		return logLevelWarn
	case "ERROR":
		return logLevelError
	case "OFF":
		return logLevelOff
	default:
		// Terraform prints a warning and uses TRACE.
		return logLevelTrace
	}
}

// terraformProviderLogLevel reproduces
// terraform/internal/logging.providerLogLevel: TF_LOG_PROVIDER wins outright
// when it is set to anything non-empty, including a value that disagrees with
// TF_LOG.
func terraformProviderLogLevel(tfLogProvider, tfLog string) logLevel {
	if strings.ToUpper(tfLogProvider) != "" {
		return parseTerraformLogLevel(tfLogProvider)
	}
	return parseTerraformLogLevel(tfLog)
}

// tunePluginSDKLoggingForEnv aligns unset TF_LOG_SDK* and
// TF_LOG_PROVIDER_GOOGLE* environment variables with Terraform's effective
// provider log level when running as a plugin child process.
//
// terraform-plugin-log/tfsdklog defaults unset TF_LOG_SDK* variables to
// hclog.NoLevel (0), which emits every TRACE and DEBUG log line (including
// runtime.Caller stack walks and JSON formatting) over the stderr pipe even
// when TF_LOG and TF_LOG_PROVIDER are unset and Terraform Core discards all of
// them.
func tunePluginSDKLoggingForEnv(lookupEnv func(string) (string, bool), setEnv func(string, string)) {
	if cookie, ok := lookupEnv(envPluginMagicCookie); !ok || cookie == "" {
		return
	}
	for _, bypassVar := range []string{envTFAcc, envTFAccLogPath, envTFTempLogPath, envTFLogPath} {
		if v, ok := lookupEnv(bypassVar); ok && v != "" {
			return
		}
	}

	tfLogProvider, _ := lookupEnv(envTFLogProvider)
	tfLog, _ := lookupEnv(envTFLog)
	level := terraformProviderLogLevel(tfLogProvider, tfLog).String()
	if level == "" || level == "TRACE" {
		return
	}

	if v, ok := lookupEnv("TF_LOG_SDK"); !ok || v == "" {
		for _, k := range sdkLogEnvVars {
			if cur, set := lookupEnv(k); !set || cur == "" {
				setEnv(k, level)
			}
		}
	}
	for _, k := range providerLogEnvVars {
		if cur, set := lookupEnv(k); !set || cur == "" {
			setEnv(k, level)
		}
	}
}

// TunePluginSDKLogging sets unset TF_LOG_SDK* and TF_LOG_PROVIDER_GOOGLE*
// environment variables to match the effective Terraform provider log level
// before tf5muxserver and tf5server initialize their root loggers.
func TunePluginSDKLogging() {
	tunePluginSDKLoggingForEnv(os.LookupEnv, func(k, v string) {
		_ = os.Setenv(k, v)
	})
}
