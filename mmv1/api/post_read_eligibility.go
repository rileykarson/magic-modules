// Copyright 2026 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PostReadEligibility summarizes whether an MMv1 resource is eligible to skip
// post-create and/or post-update Read calls (`exclude_post_create_read` and
// `exclude_post_update_read`) with 100% backwards compatibility.
type PostReadEligibility struct {
	ProductName   string `json:"product_name"`
	ResourceName  string `json:"resource_name"`
	TerraformName string `json:"terraform_name"`
	AsyncMode     string `json:"async_mode"`
	Immutable     bool   `json:"immutable"`
	Updatable     bool   `json:"updatable"`

	CreateEligible    bool     `json:"create_eligible"`
	CreateStrictClean bool     `json:"create_strict_clean"`
	CreateBlockers    []string `json:"create_blockers,omitempty"`
	CreateWarnings    []string `json:"create_warnings,omitempty"`

	UpdateEligible    bool     `json:"update_eligible"`
	UpdateStrictClean bool     `json:"update_strict_clean"`
	UpdateBlockers    []string `json:"update_blockers,omitempty"`
	UpdateWarnings    []string `json:"update_warnings,omitempty"`

	AlreadyUsesOpWaitWithResponse    bool `json:"already_uses_op_wait_with_response"`
	AlreadyUsesPostCreateSetComputed bool `json:"already_uses_post_create_set_computed"`
}

// EvaluatePostReadEligibility analyzes a resource's YAML configuration and
// template hooks to determine whether post-create and post-update Read calls
// can be safely replaced by direct flattening of the Create/Update response
// (or LRO `opRes`). If mmv1Root is non-empty, handwritten operation files are
// also checked for `<ClientNamePascal>OperationWaitTimeWithResponse`.
func (r *Resource) EvaluatePostReadEligibility(mmv1Root string) PostReadEligibility {
	res := PostReadEligibility{
		ResourceName:  r.ResourceName(),
		TerraformName: r.TerraformName(),
		AsyncMode:     "Sync",
		Immutable:     r.Immutable,
		Updatable:     r.Updatable(),
	}
	if r.ProductMetadata != nil {
		res.ProductName = r.ProductMetadata.Name
	}

	async := r.GetAsync()
	if async != nil {
		if async.IsA("OpAsync") {
			res.AsyncMode = "OpAsync"
		} else if async.IsA("PollAsync") {
			res.AsyncMode = "PollAsync"
		}
	}

	if async != nil && async.Allow("Create") && async.IsA("OpAsync") && async.Result.ResourceInsideResponse && r.HasPostCreateComputedFields() {
		res.AlreadyUsesOpWaitWithResponse = true
	}
	if r.HasPostCreateComputedFields() && (async == nil || !async.Allow("Create") || async.IsA("PollAsync")) {
		res.AlreadyUsesPostCreateSetComputed = true
	}

	// Shared blockers for both Create and Update
	if r.ExcludeRead {
		res.CreateBlockers = append(res.CreateBlockers, "exclude_read is true")
		res.UpdateBlockers = append(res.UpdateBlockers, "exclude_read is true")
	}
	if r.CustomCode.PreRead != "" {
		msg := fmt.Sprintf("custom_code.pre_read (%s) runs only in Read", r.CustomCode.PreRead)
		res.CreateBlockers = append(res.CreateBlockers, msg)
		res.UpdateBlockers = append(res.UpdateBlockers, msg)
	}
	if r.CustomCode.PostRead != "" {
		msg := fmt.Sprintf("custom_code.post_read (%s) runs only in Read", r.CustomCode.PostRead)
		res.CreateBlockers = append(res.CreateBlockers, msg)
		res.UpdateBlockers = append(res.UpdateBlockers, msg)
	}
	if r.ReadQueryParams != "" {
		msg := fmt.Sprintf("read_query_params (%s) alters Read request URL", r.ReadQueryParams)
		res.CreateBlockers = append(res.CreateBlockers, msg)
		res.UpdateBlockers = append(res.UpdateBlockers, msg)
	}

	// Create-specific blockers
	if r.CustomCode.CustomCreate != "" {
		res.CreateBlockers = append(res.CreateBlockers, fmt.Sprintf("custom_code.custom_create (%s) replaces standard Create", r.CustomCode.CustomCreate))
	}
	if r.CustomCode.PostCreate != "" {
		res.CreateBlockers = append(res.CreateBlockers, fmt.Sprintf("custom_code.post_create (%s) may mutate remote state or rely on trailing Read", r.CustomCode.PostCreate))
	}
	if async != nil && async.Allow("Create") {
		if async.IsA("PollAsync") {
			res.CreateBlockers = append(res.CreateBlockers, "PollAsync on Create does not return final resource in initial response")
		} else if async.IsA("OpAsync") {
			if !async.Result.ResourceInsideResponse {
				res.CreateBlockers = append(res.CreateBlockers, "OpAsync on Create without async.result.resource_inside_response: true")
			} else if !r.hasOperationWaitTimeWithResponse(mmv1Root) {
				res.CreateBlockers = append(res.CreateBlockers, fmt.Sprintf("missing %sOperationWaitTimeWithResponse in handwritten operation file", r.ClientNamePascal()))
			}
		}
	}

	// Create warnings
	if r.CustomCode.Decoder != "" {
		res.CreateWarnings = append(res.CreateWarnings, fmt.Sprintf("custom_code.decoder (%s)", r.CustomCode.Decoder))
	}
	if r.NestedQuery != nil {
		res.CreateWarnings = append(res.CreateWarnings, "nested_query requires unwrapping response via flattenNested")
	}
	if r.CustomCode.PreCreate != "" {
		if r.customTemplateCallsRead(mmv1Root, r.CustomCode.PreCreate) {
			res.CreateBlockers = append(res.CreateBlockers, fmt.Sprintf("custom_code.pre_create (%s) directly calls resource Read(d, meta)", r.CustomCode.PreCreate))
		} else {
			res.CreateWarnings = append(res.CreateWarnings, fmt.Sprintf("custom_code.pre_create (%s)", r.CustomCode.PreCreate))
		}
	}

	// Update-specific blockers
	if r.CustomCode.CustomUpdate != "" {
		res.UpdateBlockers = append(res.UpdateBlockers, fmt.Sprintf("custom_code.custom_update (%s) replaces standard Update", r.CustomCode.CustomUpdate))
	}
	if r.CustomCode.PostUpdate != "" {
		res.UpdateBlockers = append(res.UpdateBlockers, fmt.Sprintf("custom_code.post_update (%s) may mutate remote state or rely on trailing Read", r.CustomCode.PostUpdate))
	}
	if r.FieldSpecificUpdateMethods() {
		res.UpdateBlockers = append(res.UpdateBlockers, "field-specific update_url methods execute separate RPCs")
	}
	if !r.Immutable {
		if async != nil && async.Allow("update") {
			if async.IsA("PollAsync") {
				res.UpdateBlockers = append(res.UpdateBlockers, "PollAsync on Update does not return final resource in Update response")
			} else if async.IsA("OpAsync") {
				if !async.Result.ResourceInsideResponse {
					res.UpdateBlockers = append(res.UpdateBlockers, "OpAsync on Update without async.result.resource_inside_response: true")
				} else if !r.hasOperationWaitTimeWithResponse(mmv1Root) {
					res.UpdateBlockers = append(res.UpdateBlockers, fmt.Sprintf("missing %sOperationWaitTimeWithResponse in handwritten operation file", r.ClientNamePascal()))
				}
			}
		}
	}

	// Update warnings
	if r.CustomCode.Decoder != "" {
		res.UpdateWarnings = append(res.UpdateWarnings, fmt.Sprintf("custom_code.decoder (%s)", r.CustomCode.Decoder))
	}
	if r.NestedQuery != nil {
		res.UpdateWarnings = append(res.UpdateWarnings, "nested_query requires unwrapping response via flattenNested")
	}
	if r.CustomCode.PreUpdate != "" {
		if r.customTemplateCallsRead(mmv1Root, r.CustomCode.PreUpdate) {
			res.UpdateBlockers = append(res.UpdateBlockers, fmt.Sprintf("custom_code.pre_update (%s) directly calls resource Read(d, meta)", r.CustomCode.PreUpdate))
		} else {
			res.UpdateWarnings = append(res.UpdateWarnings, fmt.Sprintf("custom_code.pre_update (%s)", r.CustomCode.PreUpdate))
		}
	}
	if !r.Immutable && async != nil && async.IsA("OpAsync") && !async.Allow("update") {
		res.UpdateWarnings = append(res.UpdateWarnings, "resource uses OpAsync but Update is synchronous (not in async.actions)")
	}

	res.CreateEligible = len(res.CreateBlockers) == 0
	res.CreateStrictClean = res.CreateEligible && len(res.CreateWarnings) == 0
	res.UpdateEligible = len(res.UpdateBlockers) == 0
	res.UpdateStrictClean = res.UpdateEligible && len(res.UpdateWarnings) == 0

	return res
}

func (r *Resource) hasOperationWaitTimeWithResponse(mmv1Root string) bool {
	if r.AutogenAsync {
		return true
	}
	if mmv1Root == "" || r.ProductMetadata == nil {
		return true
	}
	funcName := r.ClientNamePascal() + "OperationWaitTimeWithResponse"
	serviceDir := filepath.Join(mmv1Root, "third_party", "terraform", "services", r.PackageName())
	entries, err := os.ReadDir(serviceDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(serviceDir, entry.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), funcName) {
			return true
		}
	}
	return false
}

func (r *Resource) customTemplateCallsRead(mmv1Root, relPath string) bool {
	if mmv1Root == "" || relPath == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(mmv1Root, relPath))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "Read(d, meta)")
}
