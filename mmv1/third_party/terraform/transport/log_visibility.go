package transport

import (
	"os"
	"strings"
	"sync"
)

// A provider plugin has no direct way to ask "is anybody listening at level
// X?". The plugin writes its standard library log output to stderr
// unconditionally; all filtering happens on the Terraform side. Terraform's
// go-plugin client reads that stderr and replays each line onto the logger
// returned by terraform/internal/logging.NewProviderLogger, whose level comes
// from TF_LOG_PROVIDER falling back to TF_LOG. Those variables are part of
// the environment Terraform passes to the plugin, so the provider can compute
// the same answer locally.
//
// Measured against terraform 1.16.2 and this provider's main.go, which serves
// through tf5server rather than terraform-plugin-sdk's plugin.Serve, a line
// written with log.Printf reaches the user's log at [DEBUG], not at [INFO]:
// go-plugin fails to parse the plain-text line as hclog JSON and falls back
// to logging the raw line at debug level. So the identity line below is
// visible today at TF_LOG=trace|debug|json|<invalid> and invisible at
// info|warn|error|off|unset, and the same for TF_LOG_PROVIDER.
//
// This predicate deliberately answers "visible" for INFO as well, one level
// more permissive than what was measured. The measured threshold is a
// consequence of which Serve function main.go happens to call and of
// go-plugin's JSON fallback, neither of which is a stable contract; if either
// changed so that provider lines arrived at their true level, INFO would
// become the threshold. Being too permissive costs latency in a rare
// configuration. Being too restrictive silently deletes output.
//
// Only use this to skip work whose *sole* purpose is to produce a log line.

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
	// Operator override: force the provider to behave as if INFO were
	// visible, for the case where this predicate is wrong.
	envAlwaysLogIdentity = "TF_GOOGLE_ALWAYS_LOG_IDENTITY"
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

// sdkLogLevel reproduces terraform-plugin-sdk/v2/helper/logging.LogLevel,
// which is what filters the provider's log output when it is served
// in-process by the acceptance test harness. It differs from Terraform's
// mapping in two ways that matter: it reads only TF_LOG, and it does not
// recognise "OFF" or "JSON", so both fall through to TRACE.
func sdkLogLevel(tfLog string) logLevel {
	if tfLog == "" {
		return logLevelOff
	}

	switch strings.ToUpper(tfLog) {
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
	default:
		return logLevelTrace
	}
}

// logVisibilityEnv is the subset of the environment the predicate depends on,
// extracted so it can be tested without mutating the process environment.
type logVisibilityEnv struct {
	tfLog             string
	tfLogProvider     string
	tfTempLogPath     string
	tfAcc             string
	tfAccLogPath      string
	alwaysLogIdentity string
}

func readLogVisibilityEnv() logVisibilityEnv {
	return logVisibilityEnv{
		tfLog:             os.Getenv(envTFLog),
		tfLogProvider:     os.Getenv(envTFLogProvider),
		tfTempLogPath:     os.Getenv(envTFTempLogPath),
		tfAcc:             os.Getenv(envTFAcc),
		tfAccLogPath:      os.Getenv(envTFAccLogPath),
		alwaysLogIdentity: os.Getenv(envAlwaysLogIdentity),
	}
}

// infoLogsVisible reports whether an [INFO] line written by this process has
// any chance of reaching a human or a log file.
//
// Every clause is a reason to answer yes; there is no clause that can veto
// one, so adding a listener can never make this return false.
func (e logVisibilityEnv) infoLogsVisible() bool {
	switch {
	case e.alwaysLogIdentity != "":
		// Explicit operator override.
		return true
	case e.tfAcc != "", e.tfAccLogPath != "":
		// Acceptance tests. The harness serves the provider in-process and
		// may enable TRACE by itself; never trade test diagnostics for
		// startup latency.
		return true
	case e.tfTempLogPath != "":
		// Terraform attaches a TRACE sink to this file which ignores the
		// provider logger's level.
		return true
	case terraformProviderLogLevel(e.tfLogProvider, e.tfLog) <= logLevelInfo:
		// The normal case: Terraform launched us as a plugin and will replay
		// our stderr at a level that includes INFO. Note logLevelNone (0)
		// also lands here, which is correct - hclog emits everything at
		// NoLevel.
		return true
	case sdkLogLevel(e.tfLog) <= logLevelInfo:
		// The SDK's own, slightly different mapping. Only reachable when it
		// is more permissive than Terraform's, i.e. TF_LOG=off or an
		// unrecognised TF_LOG. Being wrong here costs latency, not output.
		return true
	default:
		return false
	}
}

var (
	infoLogsVisibleOnce   sync.Once
	infoLogsVisibleCached bool
)

// InfoLogsVisible reports whether [INFO] output from this provider process is
// being collected by anything. The environment cannot change under a plugin
// process, so the answer is computed once.
//
// This exists so that expensive work performed *purely* to produce an [INFO]
// line can be skipped. Do not use it to skip work with any other effect.
func InfoLogsVisible() bool {
	infoLogsVisibleOnce.Do(func() {
		infoLogsVisibleCached = readLogVisibilityEnv().infoLogsVisible()
	})
	return infoLogsVisibleCached
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
