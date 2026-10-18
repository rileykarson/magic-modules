package registry

import (
	"runtime/debug"
	"strconv"
	"testing"
)

func TestGcTuningForEnv(t *testing.T) {
	cases := map[string]struct {
		env             map[string]string
		wantGCPercent   *int
		wantMemoryLimit *int64
	}{
		"not a plugin process": {
			env: map[string]string{},
		},
		"empty magic cookie is not a plugin process": {
			env: map[string]string{pluginMagicCookieEnvVar: ""},
		},
		"plugin process": {
			env:             map[string]string{pluginMagicCookieEnvVar: "cookie"},
			wantGCPercent:   intPtr(pluginGCPercent),
			wantMemoryLimit: int64Ptr(pluginSoftMemoryLimit),
		},
		"plugin process with tuning disabled": {
			env: map[string]string{
				pluginMagicCookieEnvVar: "cookie",
				disableGCTuningEnvVar:   "1",
			},
		},
		"GOGC set by the operator turns the tuning off entirely": {
			env: map[string]string{
				pluginMagicCookieEnvVar: "cookie",
				gcPercentEnvVar:         "50",
			},
		},
		"GOGC=off by the operator is not given a memory limit": {
			env: map[string]string{
				pluginMagicCookieEnvVar: "cookie",
				gcPercentEnvVar:         "off",
			},
		},
		"GOMEMLIMIT set by the operator wins": {
			env: map[string]string{
				pluginMagicCookieEnvVar: "cookie",
				memoryLimitEnvVar:       "512MiB",
			},
			wantGCPercent: intPtr(pluginGCPercent),
		},
		"both runtime knobs set by the operator": {
			env: map[string]string{
				pluginMagicCookieEnvVar: "cookie",
				gcPercentEnvVar:         "off",
				memoryLimitEnvVar:       "512MiB",
			},
		},
		"empty runtime knobs are treated as unset": {
			env: map[string]string{
				pluginMagicCookieEnvVar: "cookie",
				gcPercentEnvVar:         "",
				memoryLimitEnvVar:       "",
			},
			wantGCPercent:   intPtr(pluginGCPercent),
			wantMemoryLimit: int64Ptr(pluginSoftMemoryLimit),
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			lookupEnv := func(key string) (string, bool) {
				v, ok := tc.env[key]
				return v, ok
			}

			got := gcTuningForEnv(lookupEnv)

			if (got.GCPercent == nil) != (tc.wantGCPercent == nil) ||
				(got.GCPercent != nil && *got.GCPercent != *tc.wantGCPercent) {
				t.Errorf("gcTuningForEnv() GCPercent = %s, want %s", formatIntPtr(got.GCPercent), formatIntPtr(tc.wantGCPercent))
			}
			if (got.MemoryLimit == nil) != (tc.wantMemoryLimit == nil) ||
				(got.MemoryLimit != nil && *got.MemoryLimit != *tc.wantMemoryLimit) {
				t.Errorf("gcTuningForEnv() MemoryLimit = %s, want %s", formatInt64Ptr(got.MemoryLimit), formatInt64Ptr(tc.wantMemoryLimit))
			}
		})
	}
}

// TestGcTuningApplyIsInert makes sure applying an empty tuning does not touch
// the runtime, since that is what every non-plugin process gets.
func TestGcTuningApplyIsInert(t *testing.T) {
	beforeGCPercent, beforeMemoryLimit := currentGCSettings()

	gcTuning{}.apply()

	afterGCPercent, afterMemoryLimit := currentGCSettings()
	if afterGCPercent != beforeGCPercent {
		t.Errorf("empty tuning changed GOGC from %d to %d", beforeGCPercent, afterGCPercent)
	}
	if afterMemoryLimit != beforeMemoryLimit {
		t.Errorf("empty tuning changed the memory limit from %d to %d", beforeMemoryLimit, afterMemoryLimit)
	}
}

// TestGcTuningApply checks that a populated tuning reaches the runtime, so
// that a plugin process really does get the settings the table above
// describes.
func TestGcTuningApply(t *testing.T) {
	originalGCPercent, originalMemoryLimit := currentGCSettings()
	t.Cleanup(func() {
		debug.SetGCPercent(originalGCPercent)
		debug.SetMemoryLimit(originalMemoryLimit)
	})

	gcPercent := originalGCPercent + 37
	memoryLimit := originalMemoryLimit/2 + 1
	gcTuning{GCPercent: &gcPercent, MemoryLimit: &memoryLimit}.apply()

	gotGCPercent, gotMemoryLimit := currentGCSettings()
	if gotGCPercent != gcPercent {
		t.Errorf("apply() set GOGC to %d, want %d", gotGCPercent, gcPercent)
	}
	if gotMemoryLimit != memoryLimit {
		t.Errorf("apply() set the memory limit to %d, want %d", gotMemoryLimit, memoryLimit)
	}
}

// currentGCSettings reads the runtime's GC settings back. Both setters return
// the previous value and have no getter, so reading means setting the value
// back to what it already was.
func currentGCSettings() (gcPercent int, memoryLimit int64) {
	gcPercent = debug.SetGCPercent(100)
	debug.SetGCPercent(gcPercent)

	memoryLimit = debug.SetMemoryLimit(-1)
	return gcPercent, memoryLimit
}

func intPtr(i int) *int { return &i }

func int64Ptr(i int64) *int64 { return &i }

func formatIntPtr(i *int) string {
	if i == nil {
		return "<nil>"
	}
	return strconv.Itoa(*i)
}

func formatInt64Ptr(i *int64) string {
	if i == nil {
		return "<nil>"
	}
	return strconv.FormatInt(*i, 10)
}
