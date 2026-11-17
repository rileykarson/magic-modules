package dataflow

import (
	"testing"

	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
)

func TestDataflowJobVirtualUpdate(t *testing.T) {
	jobSchema := ResourceDataflowJob().Schema
	flexSchema := ResourceDataflowFlexTemplateJob().Schema

	cases := []struct {
		name        string
		changed     []string
		wantVirtual bool
		wantUpdate  bool
	}{
		{"empty diff", nil, false, false},
		{"single virtual field", []string{"skip_wait_on_job_termination"}, true, false},
		{"combined virtual fields", []string{"on_delete", "skip_wait_on_job_termination", "create_ignore_already_exists", "deletion_policy"}, true, false},
		{"labels lineage without effective_labels", []string{"labels", "terraform_labels", "deletion_policy"}, false, false},
		{"effective_labels changed", []string{"labels", "terraform_labels", "effective_labels", "deletion_policy"}, false, true},
		{"virtual plus real field", []string{"on_delete", "max_workers", "parameters"}, false, true},
	}

	for _, tc := range cases {
		d := &tpgresource.ResourceDataMock{FieldsWithHasChange: tc.changed}
		if got := resourceDataflowJobIsVirtualUpdate(d, flexSchema); got != tc.wantVirtual {
			t.Errorf("%s: resourceDataflowJobIsVirtualUpdate() = %v, want %v", tc.name, got, tc.wantVirtual)
		}
		if got := jobHasUpdate(d, jobSchema); got != tc.wantUpdate {
			t.Errorf("%s: jobHasUpdate() = %v, want %v", tc.name, got, tc.wantUpdate)
		}
		if got := flexTemplateJobHasUpdate(d, flexSchema); got != tc.wantUpdate {
			t.Errorf("%s: flexTemplateJobHasUpdate() = %v, want %v", tc.name, got, tc.wantUpdate)
		}
	}
}
