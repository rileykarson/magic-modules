/*
* Copyright 2026 Google LLC. All Rights Reserved.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */
package cmd

import (
	"magician/source"
	"os"
	"reflect"
	"testing"
)

type tgcIntegrationTestRunner struct {
	*mockRunner
}

func (r *tgcIntegrationTestRunner) ReadFile(name string) (string, error) {
	val, ok := r.fileContents[name]
	if !ok {
		return "", os.ErrNotExist
	}
	return val, nil
}

func TestExecTestTGCIntegration(t *testing.T) {
	tgcRepoPath := "/mock/dir/go/src/github.com/modular-magician/terraform-google-conversion"
	targetURL := "https://console.cloud.google.com/cloud-build/builds;region=global/build1;step=19?project=project1"
	expectedRunsWhenExecuted := []ParameterList{
		{"/mock/dir/magic-modules/.ci/magician", "git", []string{"clone", "-b", "auto-pr-123", "https://modular-magician:*******@github.com/modular-magician/terraform-google-conversion", tgcRepoPath}, map[string]string(nil)},
		{tgcRepoPath, "git", []string{"diff", "--name-only", "HEAD~1"}, map[string]string(nil)},
		{tgcRepoPath, "go", []string{"mod", "edit", "-replace", "github.com/hashicorp/terraform-provider-google-beta=github.com/modular-magician/terraform-provider-google-beta@auto-pr-123"}, map[string]string(nil)},
		{tgcRepoPath, "go", []string{"mod", "tidy"}, map[string]string(nil)},
		{tgcRepoPath, "make", []string{"build"}, map[string]string(nil)},
		{tgcRepoPath, "make", []string{"test-integration"}, map[string]string(nil)},
	}
	expectedStatusesWhenExecuted := [][]any{
		{"123", "terraform-google-conversion-test-integration", "pending", targetURL, "mm-sha"},
		{"123", "terraform-google-conversion-test-integration", "success", targetURL, "mm-sha"},
	}

	cases := []struct {
		name             string
		newSha           string
		oldSha           string
		writeNewSha      bool
		writeOldSha      bool
		expectedRuns     []ParameterList
		expectedStatuses [][]any
	}{
		{
			name:             "matching SHAs skips cloning and tests",
			newSha:           "abc123\n",
			oldSha:           "abc123",
			writeNewSha:      true,
			writeOldSha:      true,
			expectedRuns:     nil,
			expectedStatuses: nil,
		},
		{
			name:             "differing SHAs proceeds as normal",
			newSha:           "def456\n",
			oldSha:           "abc123\n",
			writeNewSha:      true,
			writeOldSha:      true,
			expectedRuns:     expectedRunsWhenExecuted,
			expectedStatuses: expectedStatusesWhenExecuted,
		},
		{
			name:             "missing old SHA file proceeds as normal",
			newSha:           "abc123\n",
			writeNewSha:      true,
			writeOldSha:      false,
			expectedRuns:     expectedRunsWhenExecuted,
			expectedStatuses: expectedStatusesWhenExecuted,
		},
		{
			name:             "empty old SHA file proceeds as normal",
			newSha:           "abc123\n",
			oldSha:           "  \n",
			writeNewSha:      true,
			writeOldSha:      true,
			expectedRuns:     expectedRunsWhenExecuted,
			expectedStatuses: expectedStatusesWhenExecuted,
		},
		{
			name:             "empty new and old SHA files proceeds as normal",
			newSha:           "  \n",
			oldSha:           "",
			writeNewSha:      true,
			writeOldSha:      true,
			expectedRuns:     expectedRunsWhenExecuted,
			expectedStatuses: expectedStatusesWhenExecuted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseRunner := NewMockRunner().(*mockRunner)
			baseRunner.cmdResults[tgcRepoPath+" git [diff --name-only HEAD~1] map[]"] = "test/services/alloydb/alloydb_cluster_generated_test.go\n"
			rnr := &tgcIntegrationTestRunner{mockRunner: baseRunner}

			if tc.writeNewSha {
				if err := rnr.WriteFile("/workspace/commitSHA_modular-magician_terraform-google-conversion.txt", tc.newSha); err != nil {
					t.Fatalf("WriteFile newSha failed: %v", err)
				}
			}
			if tc.writeOldSha {
				if err := rnr.WriteFile("/workspace/commitSHA_modular-magician_terraform-google-conversion-old.txt", tc.oldSha); err != nil {
					t.Fatalf("WriteFile oldSha failed: %v", err)
				}
			}

			gh := &mockGithub{
				calledMethods: make(map[string][][]any),
			}
			ctlr := source.NewController("/mock/dir/go", "modular-magician", "*******", rnr)

			err := execTestTGCIntegration("123", "mm-sha", "build1", "project1", "19", "terraform-google-conversion", "modular-magician", rnr, ctlr, gh)
			if err != nil {
				t.Fatalf("execTestTGCIntegration returned unexpected error: %v", err)
			}

			actualRuns, _ := rnr.Calls("Run")
			if !reflect.DeepEqual(actualRuns, tc.expectedRuns) {
				t.Errorf("Run calls = %v, want %v", actualRuns, tc.expectedRuns)
			}

			actualStatuses := gh.calledMethods["PostBuildStatus"]
			if !reflect.DeepEqual(actualStatuses, tc.expectedStatuses) {
				t.Errorf("PostBuildStatus calls = %v, want %v", actualStatuses, tc.expectedStatuses)
			}
		})
	}
}

func TestShouldRunTests(t *testing.T) {
	cases := []struct {
		name         string
		changedFiles []string
		expected     bool
	}{
		{
			name:         "relevant go file",
			changedFiles: []string{"test/services/alloydb/alloydb_cluster_generated_test.go"},
			expected:     true,
		},
		{
			name:         "non-go file",
			changedFiles: []string{"docs/supported_resources.md"},
			expected:     false,
		},
		{
			name:         "ignored directory cai2hcl",
			changedFiles: []string{"cai2hcl/services/certificatemanager/certificate.go"},
			expected:     false,
		},
		{
			name:         "ignored directory tfplan2cai",
			changedFiles: []string{"tfplan2cai/converters/google/resources/services/biglakeiceberg"},
			expected:     false,
		},
		{
			name:         "pkg/services file (ignored by default)",
			changedFiles: []string{"pkg/services/compute/compute_disk.go"},
			expected:     false,
		},
		{
			name:         "pkg/provider/provider_register_services.go file (ignored by default)",
			changedFiles: []string{"pkg/provider/provider_register_services.go"},
			expected:     false,
		},
		{
			name:         "pkg/services cai2hcl file (no longer exception)",
			changedFiles: []string{"pkg/services/compute/compute_disk_cai2hcl.go"},
			expected:     false,
		},
		{
			name:         "pkg/services tfplan2cai file (no longer exception)",
			changedFiles: []string{"pkg/services/compute/compute_disk_tfplan2cai.go"},
			expected:     false,
		},
		{
			name:         "pkg/cai2hcl file",
			changedFiles: []string{"pkg/cai2hcl/converters/convert_resource.go"},
			expected:     true,
		},
		{
			name:         "pkg/tfplan2cai file",
			changedFiles: []string{"pkg/tfplan2cai/converters/cai/convert.go"},
			expected:     true,
		},
		{
			name:         "pkg/caiasset file",
			changedFiles: []string{"pkg/caiasset/asset.go"},
			expected:     true,
		},
		{
			name:         "multiple files, one relevant",
			changedFiles: []string{"README.md", "caiasset/asset.go", "pkg/cai2hcl/converters/convert_resource.go"},
			expected:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := shouldRunTests(tc.changedFiles)
			if actual != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}
