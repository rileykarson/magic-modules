package dataflow

import (
	"context"
	"testing"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func TestDataflowJobVirtualUpdateAndHasUpdate(t *testing.T) {
	t.Parallel()

	jobRes := ResourceDataflowJob()
	flexRes := ResourceDataflowFlexTemplateJob()

	baseJobAttrs := func() map[string]string {
		return map[string]string{
			"id":                           "job-1",
			"name":                         "my-job",
			"template_gcs_path":            "gs://bucket/template",
			"temp_gcs_location":            "gs://bucket/tmp",
			"on_delete":                    "drain",
			"skip_wait_on_job_termination": "false",
			"deletion_policy":              "DELETE",
			"labels.%":                     "1",
			"labels.env":                   "dev",
			"terraform_labels.%":           "1",
			"terraform_labels.env":         "dev",
			"effective_labels.%":           "1",
			"effective_labels.env":         "dev",
		}
	}

	baseFlexAttrs := func() map[string]string {
		return map[string]string{
			"id":                           "flex-1",
			"name":                         "my-flex-job",
			"container_spec_gcs_path":      "gs://bucket/spec.json",
			"on_delete":                    "cancel",
			"skip_wait_on_job_termination": "false",
			"create_ignore_already_exists": "false",
			"deletion_policy":              "DELETE",
			"labels.%":                     "1",
			"labels.env":                   "dev",
			"terraform_labels.%":           "1",
			"terraform_labels.env":         "dev",
			"effective_labels.%":           "1",
			"effective_labels.env":         "dev",
		}
	}

	cases := []struct {
		name               string
		isFlex             bool
		diffAttrs          map[string]*terraform.ResourceAttrDiff
		wantVirtualUpdate  bool
		wantHasUpdate      bool
		wantPersistedState map[string]string
	}{
		{
			name:              "job: empty diff",
			diffAttrs:         map[string]*terraform.ResourceAttrDiff{},
			wantVirtualUpdate: false,
			wantHasUpdate:     false,
		},
		{
			name:   "job: on_delete only",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"on_delete": {Old: "drain", New: "cancel"},
			},
			wantVirtualUpdate:  true,
			wantHasUpdate:      false,
			wantPersistedState: map[string]string{"on_delete": "cancel"},
		},
		{
			name:   "job: skip_wait_on_job_termination only",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"skip_wait_on_job_termination": {Old: "false", New: "true"},
			},
			wantVirtualUpdate:  true,
			wantHasUpdate:      false,
			wantPersistedState: map[string]string{"skip_wait_on_job_termination": "true"},
		},
		{
			name:   "job: deletion_policy only",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"deletion_policy": {Old: "DELETE", New: "ABANDON"},
			},
			wantVirtualUpdate:  true,
			wantHasUpdate:      false,
			wantPersistedState: map[string]string{"deletion_policy": "ABANDON"},
		},
		{
			name:   "job: multiple virtual fields combined",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"on_delete":                    {Old: "drain", New: "cancel"},
				"skip_wait_on_job_termination": {Old: "false", New: "true"},
				"deletion_policy":              {Old: "DELETE", New: "PREVENT"},
			},
			wantVirtualUpdate: true,
			wantHasUpdate:     false,
			wantPersistedState: map[string]string{
				"on_delete":                    "cancel",
				"skip_wait_on_job_termination": "true",
				"deletion_policy":              "PREVENT",
			},
		},
		{
			name:   "job: labels lineage change without effective_labels plus deletion_policy",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"labels.%":        {Old: "1", New: "0"},
				"labels.env":      {Old: "dev", New: "", NewRemoved: true},
				"deletion_policy": {Old: "DELETE", New: "ABANDON"},
			},
			wantVirtualUpdate: false,
			wantHasUpdate:     false,
		},
		{
			name:   "job: effective_labels change plus deletion_policy triggers replacement",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"labels.env":           {Old: "dev", New: "prod"},
				"terraform_labels.env": {Old: "dev", New: "prod"},
				"effective_labels.env": {Old: "dev", New: "prod"},
				"deletion_policy":      {Old: "DELETE", New: "ABANDON"},
			},
			wantVirtualUpdate: false,
			wantHasUpdate:     true,
		},
		{
			name:   "job: real field temp_gcs_location plus on_delete triggers replacement",
			isFlex: false,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"temp_gcs_location": {Old: "gs://bucket/tmp", New: "gs://bucket/tmp2"},
				"on_delete":         {Old: "drain", New: "cancel"},
			},
			wantVirtualUpdate: false,
			wantHasUpdate:     true,
		},
		{
			name:              "flex: empty diff",
			isFlex:            true,
			diffAttrs:         map[string]*terraform.ResourceAttrDiff{},
			wantVirtualUpdate: false,
			wantHasUpdate:     false,
		},
		{
			name:   "flex: skip_wait_on_job_termination only",
			isFlex: true,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"skip_wait_on_job_termination": {Old: "false", New: "true"},
			},
			wantVirtualUpdate:  true,
			wantHasUpdate:      false,
			wantPersistedState: map[string]string{"skip_wait_on_job_termination": "true"},
		},
		{
			name:   "flex: create_ignore_already_exists plus deletion_policy plus on_delete",
			isFlex: true,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"on_delete":                    {Old: "cancel", New: "drain"},
				"skip_wait_on_job_termination": {Old: "false", New: "true"},
				"create_ignore_already_exists": {Old: "false", New: "true"},
				"deletion_policy":              {Old: "DELETE", New: "ABANDON"},
			},
			wantVirtualUpdate: true,
			wantHasUpdate:     false,
			wantPersistedState: map[string]string{
				"on_delete":                    "drain",
				"skip_wait_on_job_termination": "true",
				"create_ignore_already_exists": "true",
				"deletion_policy":              "ABANDON",
			},
		},
		{
			name:   "flex: labels lineage change without effective_labels plus deletion_policy",
			isFlex: true,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"labels.%":        {Old: "1", New: "0"},
				"labels.env":      {Old: "dev", New: "", NewRemoved: true},
				"deletion_policy": {Old: "DELETE", New: "ABANDON"},
			},
			wantVirtualUpdate: false,
			wantHasUpdate:     false,
		},
		{
			name:   "flex: container_spec_gcs_path plus virtual fields triggers replacement",
			isFlex: true,
			diffAttrs: map[string]*terraform.ResourceAttrDiff{
				"container_spec_gcs_path":      {Old: "gs://bucket/spec.json", New: "gs://bucket/spec2.json"},
				"create_ignore_already_exists": {Old: "false", New: "true"},
				"deletion_policy":              {Old: "DELETE", New: "ABANDON"},
			},
			wantVirtualUpdate: false,
			wantHasUpdate:     true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res := jobRes
			attrs := baseJobAttrs()
			if tc.isFlex {
				res = flexRes
				attrs = baseFlexAttrs()
			}

			state := &terraform.InstanceState{
				ID:         attrs["id"],
				Attributes: attrs,
			}
			diff := &terraform.InstanceDiff{
				Attributes: tc.diffAttrs,
			}

			d, err := schema.InternalMap(res.Schema).Data(state, diff)
			if err != nil {
				t.Fatalf("Data error: %v", err)
			}

			gotVirtual := resourceDataflowJobIsVirtualUpdate(d, res.Schema)
			if gotVirtual != tc.wantVirtualUpdate {
				t.Errorf("resourceDataflowJobIsVirtualUpdate() = %v, want %v", gotVirtual, tc.wantVirtualUpdate)
			}

			var gotHasUpdate bool
			if tc.isFlex {
				gotHasUpdate = flexTemplateJobHasUpdate(d, res.Schema)
			} else {
				gotHasUpdate = jobHasUpdate(d, res.Schema)
			}
			if gotHasUpdate != tc.wantHasUpdate {
				t.Errorf("hasUpdate() = %v, want %v", gotHasUpdate, tc.wantHasUpdate)
			}

			if tc.wantVirtualUpdate {
				// Virtual-only updates must short-circuit with nil error and 0 network calls,
				// persisting all updated virtual fields in state.
				if err := res.Update(d, &transport_tpg.Config{}); err != nil {
					t.Fatalf("Update() unexpected error on virtual-only change: %v", err)
				}
				gotState := d.State()
				for k, wantV := range tc.wantPersistedState {
					if gotV := gotState.Attributes[k]; gotV != wantV {
						t.Errorf("post-Update state[%q] = %q, want %q", k, gotV, wantV)
					}
				}
			}
		})
	}
}

func TestDataflowJobBatchCustomizeDiffVirtualFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := &transport_tpg.Config{}

	t.Run("google_dataflow_job batch virtual fields are in-place", func(t *testing.T) {
		t.Parallel()
		res := ResourceDataflowJob()
		state := &terraform.InstanceState{
			ID: "batch-job-1",
			Attributes: map[string]string{
				"id":                           "batch-job-1",
				"name":                         "my-batch-job",
				"type":                         "JOB_TYPE_BATCH",
				"template_gcs_path":            "gs://bucket/template",
				"temp_gcs_location":            "gs://bucket/tmp",
				"on_delete":                    "drain",
				"skip_wait_on_job_termination": "false",
				"deletion_policy":              "PREVENT",
			},
			RawConfig: cty.ObjectVal(map[string]cty.Value{
				"deletion_policy": cty.StringVal("DELETE"),
				"labels":          cty.NullVal(cty.Map(cty.String)),
			}),
			RawPlan: cty.ObjectVal(map[string]cty.Value{
				"deletion_policy": cty.StringVal("DELETE"),
				"labels":          cty.NullVal(cty.Map(cty.String)),
			}),
		}

		// 1. Changing only virtual fields on a batch job must NOT ForceNew.
		cfgVirtualOnly := terraform.NewResourceConfigRaw(map[string]interface{}{
			"name":                         "my-batch-job",
			"template_gcs_path":            "gs://bucket/template",
			"temp_gcs_location":            "gs://bucket/tmp",
			"on_delete":                    "cancel",
			"skip_wait_on_job_termination": true,
			"deletion_policy":              "DELETE",
		})
		diff, err := schema.InternalMap(res.Schema).Diff(ctx, state, cfgVirtualOnly, res.CustomizeDiff, meta, false)
		if err != nil {
			t.Fatalf("Diff error: %v", err)
		}
		if diff == nil {
			t.Fatal("expected non-nil diff for virtual field changes")
		}
		if diff.RequiresNew() {
			t.Fatalf("expected RequiresNew() == false when only virtual fields change on batch job, got diff=%#v", diff.Attributes)
		}

		// 2. Changing a non-virtual field alongside virtual fields on a batch job MUST ForceNew the non-virtual field only.
		cfgMixed := terraform.NewResourceConfigRaw(map[string]interface{}{
			"name":                         "my-batch-job",
			"template_gcs_path":            "gs://bucket/template",
			"temp_gcs_location":            "gs://bucket/tmp-new",
			"on_delete":                    "cancel",
			"skip_wait_on_job_termination": true,
			"deletion_policy":              "DELETE",
		})
		diffMixed, err := schema.InternalMap(res.Schema).Diff(ctx, state, cfgMixed, res.CustomizeDiff, meta, false)
		if err != nil {
			t.Fatalf("Diff error: %v", err)
		}
		if !diffMixed.RequiresNew() {
			t.Fatal("expected RequiresNew() == true when temp_gcs_location changes on batch job")
		}
		if !diffMixed.Attributes["temp_gcs_location"].RequiresNew {
			t.Error("expected temp_gcs_location.RequiresNew == true")
		}
		for _, vf := range []string{"on_delete", "skip_wait_on_job_termination", "deletion_policy"} {
			if diffMixed.Attributes[vf] != nil && diffMixed.Attributes[vf].RequiresNew {
				t.Errorf("expected virtual field %q RequiresNew == false", vf)
			}
		}
	})

	t.Run("google_dataflow_flex_template_job batch virtual fields are in-place", func(t *testing.T) {
		t.Parallel()
		res := ResourceDataflowFlexTemplateJob()
		state := &terraform.InstanceState{
			ID: "batch-flex-1",
			Attributes: map[string]string{
				"id":                           "batch-flex-1",
				"name":                         "my-batch-flex",
				"type":                         "JOB_TYPE_BATCH",
				"container_spec_gcs_path":      "gs://bucket/spec.json",
				"on_delete":                    "cancel",
				"skip_wait_on_job_termination": "false",
				"create_ignore_already_exists": "true",
				"deletion_policy":              "PREVENT",
			},
			RawConfig: cty.ObjectVal(map[string]cty.Value{
				"deletion_policy": cty.StringVal("DELETE"),
				"labels":          cty.NullVal(cty.Map(cty.String)),
			}),
			RawPlan: cty.ObjectVal(map[string]cty.Value{
				"deletion_policy": cty.StringVal("DELETE"),
				"labels":          cty.NullVal(cty.Map(cty.String)),
			}),
		}

		cfgVirtualOnly := terraform.NewResourceConfigRaw(map[string]interface{}{
			"name":                         "my-batch-flex",
			"container_spec_gcs_path":      "gs://bucket/spec.json",
			"on_delete":                    "drain",
			"skip_wait_on_job_termination": true,
			"create_ignore_already_exists": false,
			"deletion_policy":              "DELETE",
		})
		diff, err := schema.InternalMap(res.Schema).Diff(ctx, state, cfgVirtualOnly, res.CustomizeDiff, meta, false)
		if err != nil {
			t.Fatalf("Diff error: %v", err)
		}
		if diff == nil {
			t.Fatal("expected non-nil diff for virtual field changes")
		}
		if diff.RequiresNew() {
			t.Fatalf("expected RequiresNew() == false when only virtual fields change on batch flex job, got diff=%#v", diff.Attributes)
		}

		cfgMixed := terraform.NewResourceConfigRaw(map[string]interface{}{
			"name":                         "my-batch-flex",
			"container_spec_gcs_path":      "gs://bucket/spec-new.json",
			"on_delete":                    "drain",
			"skip_wait_on_job_termination": true,
			"create_ignore_already_exists": false,
			"deletion_policy":              "DELETE",
		})
		diffMixed, err := schema.InternalMap(res.Schema).Diff(ctx, state, cfgMixed, res.CustomizeDiff, meta, false)
		if err != nil {
			t.Fatalf("Diff error: %v", err)
		}
		if !diffMixed.RequiresNew() {
			t.Fatal("expected RequiresNew() == true when container_spec_gcs_path changes on batch flex job")
		}
		if !diffMixed.Attributes["container_spec_gcs_path"].RequiresNew {
			t.Error("expected container_spec_gcs_path.RequiresNew == true")
		}
		for _, vf := range []string{"on_delete", "skip_wait_on_job_termination", "create_ignore_already_exists", "deletion_policy"} {
			if diffMixed.Attributes[vf] != nil && diffMixed.Attributes[vf].RequiresNew {
				t.Errorf("expected virtual field %q RequiresNew == false", vf)
			}
		}
	})
}
