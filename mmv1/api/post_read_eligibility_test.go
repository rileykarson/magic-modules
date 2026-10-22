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
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/magic-modules/mmv1/api/product"
)

func TestEvaluatePostReadEligibilityAndValidate(t *testing.T) {
	t.Parallel()

	p := &Product{
		Name: "Pubsub",
		Versions: []*product.Version{
			{Name: "ga", BaseUrl: "https://pubsub.googleapis.com/v1/"},
		},
	}
	p.SetCompiler("terraformgoogle-conversion")

	t.Run("synchronous clean resource is eligible for both create and update", func(t *testing.T) {
		r := &Resource{
			Name:                  "Topic",
			Description:           "Topic resource",
			BaseUrl:               "projects/{{project}}/topics",
			UpdateVerb:            "PATCH",
			UpdateMask:            true,
			ExcludePostCreateRead: true,
			ExcludePostUpdateRead: true,
			ProductMetadata:       p,
			Properties: []*Type{
				{Name: "name", Type: "String", Required: true, Description: "Name"},
			},
		}
		r.SetDefault(p)
		if errs := r.Validate(); len(errs) != 0 {
			t.Fatalf("expected no validation errors, got: %v", errs)
		}
		elig := r.EvaluatePostReadEligibility("")
		if !elig.CreateEligible || !elig.CreateStrictClean {
			t.Fatalf("expected CreateEligible and CreateStrictClean, got: %+v", elig)
		}
		if !elig.UpdateEligible || !elig.UpdateStrictClean {
			t.Fatalf("expected UpdateEligible and UpdateStrictClean, got: %+v", elig)
		}
	})

	t.Run("OpAsync without ResourceInsideResponse fails validation and eligibility", func(t *testing.T) {
		r := &Resource{
			Name:                  "Topic",
			Description:           "Topic resource",
			BaseUrl:               "projects/{{project}}/topics",
			AutogenAsync:          true,
			ExcludePostCreateRead: true,
			ExcludePostUpdateRead: true,
			ProductMetadata:       p,
			Async: &Async{
				Type:    "OpAsync",
				Actions: []string{"create", "update", "delete"},
				OpAsync: OpAsync{
					Result: OpAsyncResult{ResourceInsideResponse: false},
				},
				Operation: &Operation{
					Timeouts: NewTimeouts(),
				},
			},
			Properties: []*Type{
				{Name: "name", Type: "String", Required: true, Description: "Name"},
			},
		}
		r.SetDefault(p)
		errs := r.Validate()
		if len(errs) < 2 {
			t.Fatalf("expected validation errors for OpAsync without resource_inside_response, got: %v", errs)
		}
		elig := r.EvaluatePostReadEligibility("")
		if elig.CreateEligible || elig.UpdateEligible {
			t.Fatalf("expected ineligible for OpAsync without resource_inside_response, got: %+v", elig)
		}
	})

	t.Run("OpAsync with ResourceInsideResponse passes validation and eligibility", func(t *testing.T) {
		r := &Resource{
			Name:                  "Topic",
			Description:           "Topic resource",
			BaseUrl:               "projects/{{project}}/topics",
			AutogenAsync:          true,
			ExcludePostCreateRead: true,
			ExcludePostUpdateRead: true,
			ProductMetadata:       p,
			Async: &Async{
				Type:    "OpAsync",
				Actions: []string{"create", "update", "delete"},
				OpAsync: OpAsync{
					Result: OpAsyncResult{ResourceInsideResponse: true},
				},
				Operation: &Operation{
					Timeouts: NewTimeouts(),
				},
			},
			Properties: []*Type{
				{Name: "name", Type: "String", Required: true, Description: "Name"},
			},
		}
		r.SetDefault(p)
		if errs := r.Validate(); len(errs) != 0 {
			t.Fatalf("expected no validation errors, got: %v", errs)
		}
		elig := r.EvaluatePostReadEligibility("")
		if !elig.CreateEligible || !elig.UpdateEligible {
			t.Fatalf("expected eligible for OpAsync with resource_inside_response, got: %+v", elig)
		}
	})

	t.Run("FieldSpecificUpdateMethods fails ExcludePostUpdateRead validation", func(t *testing.T) {
		r := &Resource{
			Name:                  "Topic",
			Description:           "Topic resource",
			BaseUrl:               "projects/{{project}}/topics",
			ExcludePostUpdateRead: true,
			ProductMetadata:       p,
			Properties: []*Type{
				{
					Name:        "kmsKeyName",
					Type:        "String",
					Description: "KMS key name",
					UpdateUrl:   "projects/{{project}}/topics/{{name}}:updateKms",
					UpdateVerb:  "POST",
					UpdateId:    "kms",
				},
			},
		}
		r.SetDefault(p)
		errs := r.Validate()
		found := false
		for _, err := range errs {
			if strings.Contains(err.Error(), "exclude_post_update_read") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected exclude_post_update_read validation error for field-specific update_url, got: %v", errs)
		}
		elig := r.EvaluatePostReadEligibility("")
		if elig.UpdateEligible {
			t.Fatalf("expected UpdateEligible=false for field-specific update_url, got: %+v", elig)
		}
	})
}
