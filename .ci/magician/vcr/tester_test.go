package vcr

import (
	"fmt"
	"magician/provider"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCollectResults(t *testing.T) {
	for _, test := range []struct {
		name     string
		output   string
		expected Result
	}{
		{
			name: "no compound tests",
			output: `--- FAIL: TestAccServiceOneResourceOne (100.00s)
--- PASS: TestAccServiceOneResourceTwo (100.00s)
--- PASS: TestAccServiceTwoResourceOne (100.00s)
--- PASS: TestAccServiceTwoResourceTwo (100.00s)
`,
			expected: Result{
				PassedTests: []string{"TestAccServiceOneResourceTwo", "TestAccServiceTwoResourceOne", "TestAccServiceTwoResourceTwo"},
				FailedTests: []string{"TestAccServiceOneResourceOne"},
			},
		},
		{
			name: "compound tests",
			output: `--- FAIL: TestAccServiceOneResourceOne (100.00s)
--- FAIL: TestAccServiceOneResourceTwo (100.00s)
    --- PASS: TestAccServiceOneResourceTwo/test_one (100.00s)
    --- FAIL: TestAccServiceOneResourceTwo/test_two (100.00s)
--- PASS: TestAccServiceTwoResourceOne (100.00s)
    --- PASS: TestAccServiceTwoResourceOne/test_one (100.00s)
    --- PASS: TestAccServiceTwoResourceOne/test_two (100.00s)
--- PASS: TestAccServiceTwoResourceTwo (100.00s)
`,
			expected: Result{
				PassedTests: []string{
					"TestAccServiceTwoResourceOne",
					"TestAccServiceTwoResourceTwo",
				},
				FailedTests: []string{"TestAccServiceOneResourceOne", "TestAccServiceOneResourceTwo"},
				PassedSubtests: []string{
					"TestAccServiceOneResourceTwo__test_one",
					"TestAccServiceTwoResourceOne__test_one",
					"TestAccServiceTwoResourceOne__test_two",
				},
				FailedSubtests: []string{"TestAccServiceOneResourceTwo__test_two"},
			},
		},
		{
			name: "build failure",
			output: `FAIL	github.com/hashicorp/terraform-provider-google-beta/google-beta/services/corebilling [build failed]
--- PASS: TestAccServiceTwoResourceOne (100.00s)
`,
			expected: Result{
				PassedTests:   []string{"TestAccServiceTwoResourceOne"},
				BuildFailures: []string{"corebilling"},
			},
		},
		{
			name: "build failure in middle",
			output: `Error replaying tests:
error running go: exit status 1
stdout:
FAIL	github.com/hashicorp/terraform-provider-google-beta/google-beta/services/corebilling [build failed]
FAIL
stderr:
go: downloading ...
`,
			expected: Result{
				BuildFailures: []string{"corebilling"},
			},
		},
	} {
		if diff := cmp.Diff(test.expected, collectResult(test.output)); diff != "" {
			t.Errorf("collectResult(%q) got unexpected diff (-want +got):\n%s", test.output, diff)
		}
	}

}

func TestAsyncCassetteUploadPath(t *testing.T) {
	vt := &Tester{
		cassetteBucket: "ci-vcr-cassettes",
	}

	wantNightly := "gs://ci-vcr-cassettes/beta/fixtures/"
	if got := vt.asyncCassetteUploadPath("main", provider.Beta); got != wantNightly {
		t.Errorf("asyncCassetteUploadPath(\"main\", Beta) = %q; want %q", got, wantNightly)
	}

	wantFallback := "gs://ci-vcr-cassettes/beta/refs/heads//fixtures/"
	if got := vt.asyncCassetteUploadPath("", provider.Beta); got != wantFallback {
		t.Errorf("asyncCassetteUploadPath(\"\", Beta) = %q; want %q", got, wantFallback)
	}

	wantPR := "gs://ci-vcr-cassettes/beta/refs/heads/auto-pr-123/fixtures/"
	if got := vt.asyncCassetteUploadPath("auto-pr-123", provider.Beta); got != wantPR {
		t.Errorf("asyncCassetteUploadPath(\"auto-pr-123\", Beta) = %q; want %q", got, wantPR)
	}
}

type fakeVCRRunner struct {
	mu          sync.Mutex
	goListOut   string
	replayOut   string
	noTestsDirs map[string]bool
	goTestCalls []string
}

func (f *fakeVCRRunner) GetCWD() string                                                   { return "" }
func (f *fakeVCRRunner) Copy(src, dest string) error                                      { return nil }
func (f *fakeVCRRunner) Mkdir(path string) error                                          { return nil }
func (f *fakeVCRRunner) RemoveAll(path string) error                                      { return nil }
func (f *fakeVCRRunner) PushDir(path string) error                                        { return nil }
func (f *fakeVCRRunner) PopDir() error                                                    { return nil }
func (f *fakeVCRRunner) ReadFile(name string) (string, error)                             { return "", nil }
func (f *fakeVCRRunner) WriteFile(name, data string) error                                { return nil }
func (f *fakeVCRRunner) Walk(root string, fn filepath.WalkFunc) error                     { return nil }
func (f *fakeVCRRunner) MustRun(name string, args []string, env map[string]string) string { return "" }

func (f *fakeVCRRunner) Run(name string, args []string, env map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if name != "go" || len(args) < 2 {
		return "", nil
	}
	switch args[0] {
	case "list":
		return f.goListOut, nil
	case "test":
		var runFlag string
		for _, a := range args {
			if strings.HasPrefix(a, "-run=") {
				runFlag = a
				break
			}
		}
		if runFlag == "-run=TestAcc" {
			return "", fmt.Errorf("exit status 1:\n%s", f.replayOut)
		}
		f.goTestCalls = append(f.goTestCalls, args[1]+" "+runFlag)
		if f.noTestsDirs[args[1]] {
			return fmt.Sprintf("testing: warning: no tests to run\nPASS\nok  \t%s\t0.01s [no tests to run]\n", args[1]), nil
		}
		testName := strings.TrimSuffix(strings.TrimPrefix(runFlag, "-run="), "$")
		return fmt.Sprintf("--- PASS: %s (1.00s)\nok  \t%s\t1.00s\n", testName, args[1]), nil
	}
	return "", nil
}

func TestRunParallel_TargetsPackagesRecordedFromRun(t *testing.T) {
	rnr := &fakeVCRRunner{
		goListOut: "github.com/hashicorp/terraform-provider-google-beta/google-beta/services/alloydb\n" +
			"github.com/hashicorp/terraform-provider-google-beta/google-beta/services/compute\n" +
			"github.com/hashicorp/terraform-provider-google-beta/google-beta/services/storage\n",
		replayOut: `=== RUN   TestAccAlloydbBackup_basic
--- FAIL: TestAccAlloydbBackup_basic (12.00s)
FAIL to clean up test resource in alloydb
=== RUN   TestAccAlloydbInstance_update
--- PASS: TestAccAlloydbInstance_update (8.00s)
ok status returned from helper
FAIL
FAIL	github.com/hashicorp/terraform-provider-google-beta/google-beta/services/alloydb	20.00s
FAIL	github.com/hashicorp/terraform-provider-google-beta/google-beta/services/broken [build failed]
=== RUN   TestAccComputeInstance_basic
--- FAIL: TestAccComputeInstance_basic (15.00s)
FAIL
FAIL	github.com/hashicorp/terraform-provider-google-beta/google-beta/services/compute	15.00s
=== RUN   TestAccStorageBucket_basic
--- PASS: TestAccStorageBucket_basic (5.00s)
ok  	github.com/hashicorp/terraform-provider-google-beta/google-beta/services/storage	(cached)
`,
	}

	vt, err := NewTester(nil, "ci-vcr-cassettes", "ci-vcr-logs", rnr, false)
	if err != nil {
		t.Fatalf("NewTester: %v", err)
	}
	vt.SetRepoPath(provider.Beta, "tpgb")
	if err := vt.FetchCassettes(provider.Beta, "main", "auto-pr-1"); err != nil {
		t.Fatalf("FetchCassettes: %v", err)
	}

	replayRes, _ := vt.Run(RunOptions{Mode: Replaying, Version: provider.Beta})
	recordRes, err := vt.RunParallel(RunOptions{
		Mode:    Recording,
		Version: provider.Beta,
		Tests:   replayRes.FailedTests,
	})
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}

	sort.Strings(rnr.goTestCalls)
	wantCalls := []string{
		"github.com/hashicorp/terraform-provider-google-beta/google-beta/services/alloydb -run=TestAccAlloydbBackup_basic$",
		"github.com/hashicorp/terraform-provider-google-beta/google-beta/services/compute -run=TestAccComputeInstance_basic$",
	}
	if diff := cmp.Diff(wantCalls, rnr.goTestCalls); diff != "" {
		t.Errorf("RunParallel go test calls diff (-want +got):\n%s", diff)
	}
	wantResult := Result{
		PassedTests: []string{"TestAccAlloydbBackup_basic", "TestAccComputeInstance_basic"},
	}
	if diff := cmp.Diff(wantResult, recordRes); diff != "" {
		t.Errorf("RunParallel Result diff (-want +got):\n%s", diff)
	}
}

func TestRunParallel_FallbackWhenTestDirNotRecorded(t *testing.T) {
	rnr := &fakeVCRRunner{
		noTestsDirs: map[string]bool{"./google-beta/services/servicetwo": true},
	}
	vt, err := NewTester(nil, "ci-vcr-cassettes", "ci-vcr-logs", rnr, false)
	if err != nil {
		t.Fatalf("NewTester: %v", err)
	}
	vt.SetRepoPath(provider.Beta, "tpgb")

	_, err = vt.RunParallel(RunOptions{
		Mode:     Recording,
		Version:  provider.Beta,
		TestDirs: []string{"./google-beta/services/serviceone", "./google-beta/services/servicetwo"},
		Tests:    []string{"TestAccUnmatched_basic"},
	})
	if err != nil {
		t.Fatalf("RunParallel: %v", err)
	}

	sort.Strings(rnr.goTestCalls)
	wantFallbackCalls := []string{
		"./google-beta/services/serviceone -run=TestAccUnmatched_basic$",
		"./google-beta/services/servicetwo -run=TestAccUnmatched_basic$",
	}
	if diff := cmp.Diff(wantFallbackCalls, rnr.goTestCalls); diff != "" {
		t.Errorf("RunParallel fallback calls diff (-want +got):\n%s", diff)
	}

	rnr.goTestCalls = nil
	_, err = vt.RunParallel(RunOptions{
		Mode:    Replaying,
		Version: provider.Beta,
		Tests:   []string{"TestAccUnmatched_basic"},
	})
	if err != nil {
		t.Fatalf("second RunParallel: %v", err)
	}
	wantCachedCalls := []string{
		"./google-beta/services/serviceone -run=TestAccUnmatched_basic$",
	}
	if diff := cmp.Diff(wantCachedCalls, rnr.goTestCalls); diff != "" {
		t.Errorf("second RunParallel cached calls diff (-want +got):\n%s", diff)
	}
}
