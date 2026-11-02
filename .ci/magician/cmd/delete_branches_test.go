package cmd

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFetchPRNumber(t *testing.T) {
	sb := newSandbox(t)
	sb.RequireAllowlist()
	gh := &mockGithub{
		calledMethods: make(map[string][][]any),
		commitMessage: "Add `additional_group_keys` attribute to `google_cloud_identity_group` resource (#9217)\n\n* Add `additional_group_keys` attribute to `google_cloud_identity_group` resource\n\n* Update acceptance test to check for attribute\n\n* Fix test check\n\n* Add `output: true` to nested properties in output field\n\nSigned-off-by: Modular Magician <magic-modules@google.com>",
	}

	prNumber, err := fetchPRNumber("49d3741f9d4d810a0a4768363bb8498afa21c688", "main", sb.Runner, gh)
	if err != nil {
		t.Errorf("error fetching PR number: %s", err)
	}

	if prNumber != "9217" {
		t.Errorf("PR number is %s, expected 9217", prNumber)
	}

	// Verify GitHub API was called
	if calls, ok := gh.calledMethods["GetCommitMessage"]; !ok || len(calls) == 0 {
		t.Errorf("Expected GetCommitMessage to be called")
	} else {
		args := calls[0]
		if len(args) != 3 {
			t.Errorf("Expected GetCommitMessage to be called with 3 arguments, got %d", len(args))
		} else {
			if args[0] != "GoogleCloudPlatform" {
				t.Errorf("Expected owner to be 'GoogleCloudPlatform', got '%s'", args[0])
			}
			if args[1] != "magic-modules" {
				t.Errorf("Expected repo to be 'magic-modules', got '%s'", args[1])
			}
			if args[2] != "49d3741f9d4d810a0a4768363bb8498afa21c688" {
				t.Errorf("Expected SHA to be '49d3741f9d4d810a0a4768363bb8498afa21c688', got '%s'", args[2])
			}
		}
	}
}

func TestDeleteBranches(t *testing.T) {
	mr := NewMockRunner()
	if err := deleteBranches("19408", "token", mr); err != nil {
		t.Fatalf("deleteBranches() unexpected error: %v", err)
	}
	calls, ok := mr.Calls("Run")
	if !ok {
		t.Fatal("expected Run to be called")
	}
	if len(calls) != len(repoList) {
		t.Fatalf("got %d Run calls, want %d", len(calls), len(repoList))
	}
	for i, repo := range repoList {
		wantArgs := []string{
			"push",
			fmt.Sprintf("https://modular-magician:token@github.com/modular-magician/%s", repo),
			":auto-pr-19408",
			":auto-pr-19408-old",
		}
		gotArgs, ok := calls[i][2].([]string)
		if !ok || !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Errorf("call %d args = %v, want %v", i, calls[i][2], wantArgs)
		}
	}
}

func TestDeleteBranchWithRetry(t *testing.T) {
	oldBackoffs := pushRetryBackoffs
	pushRetryBackoffs = []time.Duration{0, 0, 0}
	defer func() { pushRetryBackoffs = oldBackoffs }()

	t.Run("succeeds first try", func(t *testing.T) {
		rnr := &flakyPushRunner{MockRunner: NewMockRunner(), failures: 0}
		if err := deleteBranchWithRetry(rnr, "https://example.com/repo", ":auto-pr-19408", ":auto-pr-19408-old"); err != nil {
			t.Fatalf("deleteBranchWithRetry() unexpected error: %v", err)
		}
		if rnr.pushCalls != 1 {
			t.Errorf("got %d push calls, want 1", rnr.pushCalls)
		}
	})

	t.Run("succeeds after transient failures", func(t *testing.T) {
		rnr := &flakyPushRunner{MockRunner: NewMockRunner(), failures: 2}
		if err := deleteBranchWithRetry(rnr, "https://example.com/repo", ":auto-pr-19408", ":auto-pr-19408-old"); err != nil {
			t.Fatalf("deleteBranchWithRetry() unexpected error: %v", err)
		}
		if rnr.pushCalls != 3 {
			t.Errorf("got %d push calls, want 3", rnr.pushCalls)
		}
	})

	t.Run("ignores remote ref does not exist when both refs are missing", func(t *testing.T) {
		rnr := &missingBranchPushRunner{
			MockRunner:      NewMockRunner(),
			missingBranches: map[string]bool{"auto-pr-19408": true, "auto-pr-19408-old": true},
		}
		if err := deleteBranchWithRetry(rnr, "https://example.com/repo", ":auto-pr-19408", ":auto-pr-19408-old"); err != nil {
			t.Fatalf("deleteBranchWithRetry() unexpected error: %v", err)
		}
		if rnr.pushCalls != 1 {
			t.Errorf("got %d push calls, want 1", rnr.pushCalls)
		}
	})

	t.Run("deletes remaining ref when only one ref does not exist", func(t *testing.T) {
		rnr := &missingBranchPushRunner{
			MockRunner:      NewMockRunner(),
			missingBranches: map[string]bool{"auto-pr-19408-old": true},
		}
		if err := deleteBranchWithRetry(rnr, "https://example.com/repo", ":auto-pr-19408", ":auto-pr-19408-old"); err != nil {
			t.Fatalf("deleteBranchWithRetry() unexpected error: %v", err)
		}
		if rnr.pushCalls != 2 {
			t.Fatalf("got %d push calls, want 2", rnr.pushCalls)
		}
		wantSecondCall := []string{"push", "https://example.com/repo", ":auto-pr-19408"}
		if !reflect.DeepEqual(rnr.pushArgs[1], wantSecondCall) {
			t.Errorf("second push call args = %v, want %v", rnr.pushArgs[1], wantSecondCall)
		}
	})

	t.Run("ignores remote ref does not exist for single ref without retrying", func(t *testing.T) {
		rnr := &missingBranchPushRunner{
			MockRunner:      NewMockRunner(),
			missingBranches: map[string]bool{"auto-pr-19408": true},
		}
		if err := deleteBranchWithRetry(rnr, "https://example.com/repo", ":auto-pr-19408"); err != nil {
			t.Fatalf("deleteBranchWithRetry() unexpected error: %v", err)
		}
		if rnr.pushCalls != 1 {
			t.Errorf("got %d push calls, want 1", rnr.pushCalls)
		}
	})

	t.Run("fails after exhausting retries", func(t *testing.T) {
		rnr := &flakyPushRunner{MockRunner: NewMockRunner(), failures: 10}
		err := deleteBranchWithRetry(rnr, "https://example.com/repo", ":auto-pr-19408", ":auto-pr-19408-old")
		if err == nil || !strings.Contains(err.Error(), "commit_refs") {
			t.Fatalf("expected commit_refs error, got: %v", err)
		}
		if rnr.pushCalls != 4 {
			t.Errorf("got %d push calls, want 4", rnr.pushCalls)
		}
	})
}

type missingBranchPushRunner struct {
	MockRunner
	missingBranches map[string]bool
	pushCalls       int
	pushArgs        [][]string
}

func (r *missingBranchPushRunner) Run(name string, args []string, env map[string]string) (string, error) {
	if name == "git" && len(args) > 0 && args[0] == "push" {
		r.pushCalls++
		r.pushArgs = append(r.pushArgs, args)
		var errs []string
		for _, arg := range args[2:] {
			branch := strings.TrimPrefix(arg, ":")
			if r.missingBranches[branch] {
				errs = append(errs, fmt.Sprintf("error: unable to delete '%s': remote ref does not exist", branch))
			}
		}
		if len(errs) > 0 {
			return "", errors.New(strings.Join(errs, "\n"))
		}
		return "", nil
	}
	return r.MockRunner.Run(name, args, env)
}
