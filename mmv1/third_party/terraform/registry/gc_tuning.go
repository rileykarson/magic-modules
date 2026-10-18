package registry

import (
	"os"
	"runtime/debug"
)

// A provider plugin process is short lived: Terraform starts it, runs one
// operation against it and then kills it. Nearly everything the provider
// allocates while it builds ~1500 resource schemas and answers
// GetProviderSchema stays reachable for the lifetime of the process, so the
// garbage collector cycles the default GOGC=100 pacer triggers during startup
// reclaim almost nothing while costing a meaningful amount of CPU (measured at
// 5 GC cycles before main() even runs). Trading some peak RSS for that CPU is
// worthwhile here in a way it would not be for a long running server.
//
// The tuning below is deliberately limited to processes that Terraform itself
// launched as a plugin, so test binaries and other tools that link the
// provider as a library keep the standard Go defaults, and it never overrides
// GOGC or GOMEMLIMIT if the operator set them.
const (
	// pluginMagicCookieEnvVar is the go-plugin handshake environment variable
	// that Terraform sets on provider processes it starts. terraform-plugin-go's
	// tf5server refuses to serve without it, so its presence is a reliable
	// signal that this process is a provider plugin.
	pluginMagicCookieEnvVar = "TF_PLUGIN_MAGIC_COOKIE"

	// disableGCTuningEnvVar opts a plugin process out of the tuning entirely.
	disableGCTuningEnvVar = "TF_GOOGLE_DISABLE_GC_TUNING"

	// gcPercentEnvVar and memoryLimitEnvVar are the standard Go runtime knobs.
	// If either is set the operator is driving the GC themselves and we leave
	// that setting alone.
	gcPercentEnvVar   = "GOGC"
	memoryLimitEnvVar = "GOMEMLIMIT"

	// pluginGCPercent relaxes the GC pacer. Interleaved A/B medians, plan of a
	// single resource: 1.585s wall / 2.58s CPU at the default 100, 1.350s /
	// 2.11s at 200, 1.355s / 2.16s at 400. Anything above 200 buys nothing at
	// the size of configuration this is aimed at, and costs memory at larger
	// ones - a 1000 resource plan peaks at 189MB of provider RSS at 100, 239MB
	// at 200 and 330MB at 400, for 5.59s against 5.53s of wall. 200 is the
	// point where the curve flattens on time but has not yet turned upwards on
	// memory. Turning the collector off entirely is both slower than this and
	// an order of magnitude heavier at 1000 resources.
	pluginGCPercent = 200

	// pluginSoftMemoryLimit is a backstop, not an operating point: with
	// pluginGCPercent above, a 1000 resource plan peaks around 240MB, well
	// under it. It exists so that a workload far larger than anything measured
	// here gets GC pressure rather than unbounded growth.
	pluginSoftMemoryLimit int64 = 2 << 30 // 2 GiB
)

// gcTuning describes GC settings to apply. A nil field means "leave the
// runtime default (or the operator's environment variable) in place".
type gcTuning struct {
	GCPercent   *int
	MemoryLimit *int64
}

// gcTuningForEnv returns the GC tuning appropriate for a process whose
// environment is described by lookupEnv (os.LookupEnv outside of tests).
func gcTuningForEnv(lookupEnv func(string) (string, bool)) gcTuning {
	var tuning gcTuning

	if cookie, ok := lookupEnv(pluginMagicCookieEnvVar); !ok || cookie == "" {
		// Not running as a Terraform plugin: this is a test binary, the
		// diff processor, or another tool linking the provider as a library.
		return tuning
	}
	if disabled, ok := lookupEnv(disableGCTuningEnvVar); ok && disabled != "" {
		return tuning
	}

	if gogc, ok := lookupEnv(gcPercentEnvVar); ok && gogc != "" {
		// The operator is pacing the GC themselves. Leave the runtime exactly
		// as they configured it, including not imposing the memory limit
		// below: that limit exists only as a backstop for the relaxed pacing
		// this function would otherwise install, and applying it on its own
		// would quietly contradict a setting like GOGC=off.
		return tuning
	}

	gcPercent := pluginGCPercent
	tuning.GCPercent = &gcPercent

	if memLimit, ok := lookupEnv(memoryLimitEnvVar); !ok || memLimit == "" {
		softLimit := pluginSoftMemoryLimit
		tuning.MemoryLimit = &softLimit
	}

	return tuning
}

// apply installs the tuning on the Go runtime.
func (t gcTuning) apply() {
	// The backstop goes in before the relaxed pacing, so the process is never
	// briefly running with relaxed pacing and no limit.
	if t.MemoryLimit != nil {
		debug.SetMemoryLimit(*t.MemoryLimit)
	}
	if t.GCPercent != nil {
		debug.SetGCPercent(*t.GCPercent)
	}
}

// This init runs before the init of any package that imports the registry,
// which includes every generated service package. That ordering matters: the
// service packages build their resource schemas from their own init functions,
// and those allocations are the ones we want to keep out of the GC's way.
func init() {
	gcTuningForEnv(os.LookupEnv).apply()
}
