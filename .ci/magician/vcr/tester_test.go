package vcr

import (
	"magician/provider"
	"path/filepath"
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

type fakeExecRunner struct {
	cwd     string
	runArgs [][]string
}

func (f *fakeExecRunner) GetCWD() string                               { return f.cwd }
func (f *fakeExecRunner) Copy(src, dest string) error                  { return nil }
func (f *fakeExecRunner) Mkdir(path string) error                      { return nil }
func (f *fakeExecRunner) RemoveAll(path string) error                  { return nil }
func (f *fakeExecRunner) PushDir(path string) error                    { return nil }
func (f *fakeExecRunner) PopDir() error                                { return nil }
func (f *fakeExecRunner) ReadFile(name string) (string, error)         { return "", nil }
func (f *fakeExecRunner) WriteFile(name, data string) error            { return nil }
func (f *fakeExecRunner) Walk(root string, fn filepath.WalkFunc) error { return nil }
func (f *fakeExecRunner) MustRun(name string, args []string, env map[string]string) string {
	out, _ := f.Run(name, args, env)
	return out
}
func (f *fakeExecRunner) Run(name string, args []string, env map[string]string) (string, error) {
	f.runArgs = append(f.runArgs, append([]string{name}, args...))
	return "", nil
}

func TestFetchCassettesAndRefValidation(t *testing.T) {
	cases := []struct {
		name       string
		baseBranch string
		head       string
		wantErr    bool
		wantCalls  [][]string
	}{
		{
			name:       "main base branch fetches main and head cassettes",
			baseBranch: "main",
			head:       "auto-pr-123",
			wantCalls: [][]string{
				{"gcloud", "storage", "cp", "gs://ci-vcr-cassettes/beta/fixtures/*", "/base/cassettes/beta"},
				{"gcloud", "storage", "cp", "gs://ci-vcr-cassettes/beta/refs/heads/auto-pr-123/fixtures/*", "/base/cassettes/beta"},
			},
		},
		{
			name:       "feature release branch fetches main, branch, and head cassettes",
			baseBranch: "FEATURE-BRANCH-major-release-8.0.0",
			head:       "auto-pr-123",
			wantCalls: [][]string{
				{"gcloud", "storage", "cp", "gs://ci-vcr-cassettes/beta/fixtures/*", "/base/cassettes/beta"},
				{"gcloud", "storage", "cp", "gs://ci-vcr-cassettes/beta/refs/branches/FEATURE-BRANCH-major-release-8.0.0/fixtures/*", "/base/cassettes/beta"},
				{"gcloud", "storage", "cp", "gs://ci-vcr-cassettes/beta/refs/heads/auto-pr-123/fixtures/*", "/base/cassettes/beta"},
			},
		},
		{
			name:       "rejects feature branch with wildcard",
			baseBranch: "FEATURE-BRANCH-foo/*",
			head:       "auto-pr-123",
			wantErr:    true,
		},
		{
			name:       "rejects non-release or option-injected base branch",
			baseBranch: "--evil",
			head:       "auto-pr-123",
			wantErr:    true,
		},
		{
			name:       "rejects invalid head ref with path traversal",
			baseBranch: "main",
			head:       "../evil",
			wantErr:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rnr := &fakeExecRunner{cwd: "/base"}
			vt, err := NewTester(map[string]string{}, "ci-vcr-cassettes", "ci-vcr-logs", rnr, false)
			if err != nil {
				t.Fatalf("NewTester() error: %v", err)
			}
			err = vt.FetchCassettes(provider.Beta, tc.baseBranch, tc.head)
			if (err != nil) != tc.wantErr {
				t.Fatalf("FetchCassettes(%q, %q) error = %v, wantErr %v", tc.baseBranch, tc.head, err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.wantCalls, rnr.runArgs); diff != "" {
				t.Errorf("FetchCassettes(%q, %q) commands diff (-want +got):\n%s", tc.baseBranch, tc.head, diff)
			}
		})
	}

	t.Run("UploadCassettes and UploadLogs reject invalid head refs", func(t *testing.T) {
		rnr := &fakeExecRunner{cwd: "/base"}
		vt, err := NewTester(map[string]string{}, "ci-vcr-cassettes", "ci-vcr-logs", rnr, false)
		if err != nil {
			t.Fatalf("NewTester() error: %v", err)
		}
		if err := vt.UploadCassettes("../evil", provider.Beta); err == nil {
			t.Error("UploadCassettes(../evil) succeeded, want error")
		}
		if err := vt.uploadOneCassetteFile("-flag", provider.Beta, "/tmp/f"); err == nil {
			t.Error("uploadOneCassetteFile(-flag) succeeded, want error")
		}
		if err := vt.UploadLogs(UploadLogsOptions{Head: "../evil", Mode: Replaying, Version: provider.Beta}); err == nil {
			t.Error("UploadLogs(../evil) succeeded, want error")
		}
		if len(rnr.runArgs) != 0 {
			t.Errorf("expected 0 gcloud calls on invalid head refs, got %v", rnr.runArgs)
		}
	})
}
