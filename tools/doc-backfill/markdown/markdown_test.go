package markdown

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func findDocsRDir(t *testing.T) string {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "..", "mmv1", "third_party", "terraform", "website", "docs", "r"),
		filepath.Join("..", "..", "mmv1", "third_party", "terraform", "website", "docs", "r"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	t.Fatalf("could not locate mmv1/third_party/terraform/website/docs/r from cwd")
	return ""
}

func TestDeriveResourceIdentity(t *testing.T) {
	cases := []struct {
		filename string
		wantName string
		wantIAM  bool
		wantTmpl bool
	}{
		{
			filename: "compute_instance.html.markdown",
			wantName: "google_compute_instance",
			wantIAM:  false,
			wantTmpl: false,
		},
		{
			filename: "/path/to/google_project.html.markdown",
			wantName: "google_project",
			wantIAM:  false,
			wantTmpl: false,
		},
		{
			filename: "google_project_iam_custom_role.html.markdown",
			wantName: "google_project_iam_custom_role",
			wantIAM:  false,
			wantTmpl: false,
		},
		{
			filename: "storage_bucket_iam.html.markdown",
			wantName: "google_storage_bucket_iam",
			wantIAM:  true,
			wantTmpl: false,
		},
		{
			filename: "google_folder_iam.html.markdown",
			wantName: "google_folder_iam",
			wantIAM:  true,
			wantTmpl: false,
		},
		{
			filename: "tags_tag_binding_collection.html.markdown.tmpl",
			wantName: "google_tags_tag_binding_collection",
			wantIAM:  false,
			wantTmpl: true,
		},
	}

	for _, tc := range cases {
		gotName, gotIAM, gotTmpl := deriveResourceIdentity(tc.filename)
		if gotName != tc.wantName || gotIAM != tc.wantIAM || gotTmpl != tc.wantTmpl {
			t.Errorf("deriveResourceIdentity(%q) = (%q, %v, %v), want (%q, %v, %v)",
				tc.filename, gotName, gotIAM, gotTmpl, tc.wantName, tc.wantIAM, tc.wantTmpl)
		}
	}
}

func TestParseTemplateBetaAndNormalization(t *testing.T) {
	src := `{{- if ne $.TargetVersionName "ga" }}
---
subcategory: "Test Service"
description: |-
  Manages a test widget resource
  across multiple regions.
---

# google_test_widget

Manages a test widget resource.

~> **Warning:** This resource is in beta, and should be used with the terraform-provider-google-beta provider.
See [Provider Versions](../guides/provider_versions.html.markdown) for more details on beta resources.

More overview details here.

## Example Usage

` + "```hcl\nresource \"google_test_widget\" \"w\" {}\n```" + `

## Argument Reference

The following arguments are supported:

* ` + "`name`" + ` -
  (Required)
  The widget name.

- - -

* ` + "`secret_wo`" + ` - (Optional, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The secret value.
  ~> **Note:** One of ` + "`secret`" + ` or ` + "`secret_wo`" + ` can only be set.

* ` + "`legacy_flag`" + ` - (Optional, [Beta](../guides/provider_versions.html.markdown), Deprecated) Old flag.
  * ` + "`A`" + ` - Option A
    * ` + "`A1`" + ` - Sub-option A1

* ` + "`empty_arg_block`" + ` - (Optional) Structure is [documented below](#nested_empty_arg_block).

* ` + "`config`" + ` - (Optional) Widget config. The config structure is [documented below](#nested_config). Keep this sentence.

<a name="nested_empty_arg_block"></a>The ` + "`empty_arg_block`" + ` block supports:

- ` + "`child`" + ` - (Required) Child value.

<a name="nested_config"></a>The ` + "`config`" + ` block supports:

* ` + "`items`" + ` - (Optional) See [below for nested schema](#nested_items).

<a name="nested_items"></a>The ` + "`items`" + ` block supports:

* ` + "`size`" + ` - (Optional) Size in GiB.

## Attributes Reference

In addition to the arguments listed above, the following computed attributes are exported:

* ` + "`empty_arg_block`" + ` - Exported summary of empty_arg_block when argument description was empty.

* ` + "`config`" + ` - Should NOT overwrite non-empty argument description.

* ` + "`network_interface.0.access_config.*.nat_ip`" + ` - [Beta](../guides/provider_versions.html.markdown) The allocated NAT IP.
{{- end }}
`

	doc, err := Parse("test_widget.html.markdown.tmpl", []byte(src))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if doc.ResourceName != "google_test_widget" {
		t.Errorf("ResourceName = %q, want google_test_widget", doc.ResourceName)
	}
	if !doc.IsTemplate {
		t.Errorf("IsTemplate = false, want true")
	}
	if !doc.IsBetaOnly {
		t.Errorf("IsBetaOnly = false, want true")
	}
	if doc.Subcategory != "Test Service" {
		t.Errorf("Subcategory = %q, want %q", doc.Subcategory, "Test Service")
	}
	wantPageDesc := "Manages a test widget resource\nacross multiple regions."
	if doc.PageDescription != wantPageDesc {
		t.Errorf("PageDescription = %q, want %q", doc.PageDescription, wantPageDesc)
	}
	wantOverview := "Manages a test widget resource.\n\nMore overview details here."
	if doc.Overview != wantOverview {
		t.Errorf("Overview = %q, want %q", doc.Overview, wantOverview)
	}

	// Check multiline Required badge on line after bullet
	nameField := doc.Fields["name"]
	if nameField == nil || !nameField.Required || nameField.CleanDescription != "The widget name." {
		t.Errorf("name field = %+v", nameField)
	}

	// Check WriteOnly + callout preservation
	woField := doc.Fields["secret_wo"]
	if woField == nil || !woField.Optional || !woField.WriteOnly {
		t.Errorf("secret_wo flags unexpected: %+v", woField)
	}
	wantWO := "The secret value.\n~> **Note:** One of `secret` or `secret_wo` can only be set."
	if woField.CleanDescription != wantWO {
		t.Errorf("secret_wo CleanDescription = %q, want %q", woField.CleanDescription, wantWO)
	}

	// Check Beta + Deprecated + nested list relative indentation preservation
	legField := doc.Fields["legacy_flag"]
	if legField == nil || !legField.Optional || !legField.Beta || !legField.Deprecated {
		t.Errorf("legacy_flag flags unexpected: %+v", legField)
	}
	wantLeg := "Old flag.\n* `A` - Option A\n  * `A1` - Sub-option A1"
	if legField.CleanDescription != wantLeg {
		t.Errorf("legacy_flag CleanDescription = %q, want %q", legField.CleanDescription, wantLeg)
	}

	// Check pointer stripping without mangling adjacent sentences
	cfgField := doc.Fields["config"]
	if cfgField == nil || cfgField.Section != "argument" {
		t.Fatalf("config field = %+v", cfgField)
	}
	if cfgField.CleanDescription != "Widget config. Keep this sentence." {
		t.Errorf("config CleanDescription = %q, want %q", cfgField.CleanDescription, "Widget config. Keep this sentence.")
	}

	// Check fallback to Attributes Reference when Argument Reference CleanDescription was empty
	emptyArg := doc.Fields["empty_arg_block"]
	if emptyArg == nil || emptyArg.Section != "attribute" || emptyArg.CleanDescription != "Exported summary of empty_arg_block when argument description was empty." {
		t.Errorf("empty_arg_block = %+v", emptyArg)
	}
	if child := doc.Fields["empty_arg_block.child"]; child == nil || !child.Required || child.CleanDescription != "Child value." {
		t.Errorf("empty_arg_block.child = %+v", child)
	}

	// Check deep nested block resolution
	if sz := doc.Fields["config.items.size"]; sz == nil || sz.Name != "size" || sz.CleanDescription != "Size in GiB." {
		t.Errorf("config.items.size = %+v", sz)
	}

	// Check .0. and .*. normalization + standalone leading [Beta](...) in Attributes Reference
	nat := doc.Fields["network_interface.access_config.nat_ip"]
	if nat == nil || !nat.Output || !nat.Beta || nat.Name != "nat_ip" || nat.CleanDescription != "The allocated NAT IP." {
		t.Errorf("network_interface.access_config.nat_ip = %+v", nat)
	}
}

func TestParseRealDocsCorpus(t *testing.T) {
	docsDir := findDocsRDir(t)

	docs1, err := ParseDir(docsDir)
	if err != nil {
		t.Fatalf("ParseDir run 1 failed: %v", err)
	}
	if len(docs1) < 135 {
		t.Fatalf("expected at least 135 resource docs, got %d", len(docs1))
	}

	// Determinism check across repeated full-directory parses.
	docs2, err := ParseDir(docsDir)
	if err != nil {
		t.Fatalf("ParseDir run 2 failed: %v", err)
	}
	if !reflect.DeepEqual(docs1, docs2) {
		t.Fatalf("ParseDir output is non-deterministic across repeated runs")
	}

	// Global quality invariants across all 135 docs (~5,000 fields).
	leftoverBadgeRe := regexp.MustCompile(`(?i)^\s*(?:\((?:required|optional|computed|output|beta|deprecated|write[- ]only|read[- ]only)|\[beta\])`)
	providerVersionsRe := regexp.MustCompile(`provider_versions\.html`)
	structuralPtrArtifactRe := regexp.MustCompile(`(?i)\bStructure is \[documented below\]|\bSee \[below for nested schema\]|<a\s+name=`)

	totalFields := 0
	for resName, doc := range docs1 {
		if doc.ResourceName != resName {
			t.Errorf("map key %q != doc.ResourceName %q", resName, doc.ResourceName)
		}
		if strings.HasPrefix(doc.ResourceName, "google_google_") {
			t.Errorf("double google_ prefix in %q", doc.ResourceName)
		}
		if doc.Subcategory == "" {
			t.Errorf("%s: empty Subcategory", resName)
		}
		if doc.Overview == "" {
			t.Errorf("%s: empty Overview", resName)
		}
		if strings.HasPrefix(doc.Overview, "# ") {
			t.Errorf("%s: H1 title was not stripped from Overview", resName)
		}
		if len(doc.Fields) == 0 {
			t.Errorf("%s: parsed 0 fields", resName)
		}

		for path, f := range doc.Fields {
			totalFields++
			if f.Path != path {
				t.Errorf("%s: map key %q != FieldDoc.Path %q", resName, path, f.Path)
			}
			if strings.Contains(path, ".0.") || strings.Contains(path, ".*.") {
				t.Errorf("%s.%s: unnormalized index segment in path", resName, path)
			}
			c := f.CleanDescription
			if leftoverBadgeRe.MatchString(c) {
				t.Errorf("%s.%s: leftover leading badge in CleanDescription: %q", resName, path, c)
			}
			if providerVersionsRe.MatchString(c) {
				t.Errorf("%s.%s: leftover provider_versions link in CleanDescription: %q", resName, path, c)
			}
			if structuralPtrArtifactRe.MatchString(c) {
				t.Errorf("%s.%s: leftover structural pointer/anchor in CleanDescription: %q", resName, path, c)
			}
			if strings.TrimSpace(c) == "" && c != "" {
				t.Errorf("%s.%s: whitespace-only CleanDescription %q", resName, path, c)
			}
			for _, line := range strings.Split(c, "\n") {
				if strings.HasSuffix(line, " ") || strings.HasSuffix(line, "\t") {
					t.Errorf("%s.%s: trailing whitespace on line %q", resName, path, line)
				}
			}
		}
	}
	t.Logf("Successfully verified %d resource docs with %d total canonical fields", len(docs1), totalFields)

	// Spot-check 1: google_compute_instance
	t.Run("spotcheck_compute_instance", func(t *testing.T) {
		d := docs1["google_compute_instance"]
		if d == nil {
			t.Fatal("missing google_compute_instance")
		}
		if d.IsIAM || d.IsTemplate || d.IsBetaOnly {
			t.Errorf("unexpected flags: IAM=%v Tmpl=%v Beta=%v", d.IsIAM, d.IsTemplate, d.IsBetaOnly)
		}
		if f := d.Fields["boot_disk.initialize_params.size"]; f == nil || !f.Optional || f.Name != "size" || !strings.Contains(f.CleanDescription, "size of the image in gigabytes") {
			t.Errorf("boot_disk.initialize_params.size = %+v", f)
		}
		if f := d.Fields["network_interface.access_config.nat_ip"]; f == nil || f.Section != "argument" && f.Section != "attribute" {
			t.Errorf("missing normalized network_interface.access_config.nat_ip: %+v", f)
		}
		if f := d.Fields["network_interface.access_config.security_policy"]; f == nil || !f.Beta || !f.Output || strings.Contains(f.CleanDescription, "Beta") {
			t.Errorf("network_interface.access_config.security_policy = %+v", f)
		}
		if f := d.Fields["network_performance_config"]; f == nil || !f.Optional || !f.Beta || !strings.HasPrefix(f.CleanDescription, "Configures network performance settings") {
			t.Errorf("network_performance_config = %+v", f)
		}
		// Multiline preservation check on metadata
		if f := d.Fields["metadata"]; f == nil || !strings.Contains(f.CleanDescription, "\n\n-> Depending on the OS") {
			t.Errorf("metadata multiline callout not preserved: %+v", f)
		}
	})

	// Spot-check 2: google_container_cluster
	t.Run("spotcheck_container_cluster", func(t *testing.T) {
		d := docs1["google_container_cluster"]
		if d == nil {
			t.Fatal("missing google_container_cluster")
		}
		if f := d.Fields["tpu_config"]; f == nil || !f.Optional || !f.Beta || !f.Deprecated || strings.Contains(f.CleanDescription, "documented below") {
			t.Errorf("tpu_config = %+v", f)
		}
		if f := d.Fields["secret_manager_config.rotation_config.rotation_interval"]; f == nil || !f.Optional {
			t.Errorf("secret_manager_config.rotation_config.rotation_interval = %+v", f)
		}
		if f := d.Fields["node_config.containerd_config.private_registry_access_config.certificate_authority_domain_config.fqdns"]; f == nil || !f.Required {
			t.Errorf("deep indented containerd_config fqdns = %+v", f)
		}
	})

	// Spot-check 3: google_storage_notification
	t.Run("spotcheck_storage_notification", func(t *testing.T) {
		d := docs1["google_storage_notification"]
		if d == nil {
			t.Fatal("missing google_storage_notification")
		}
		if d.Subcategory != "Cloud Storage" {
			t.Errorf("Subcategory = %q", d.Subcategory)
		}
		if f := d.Fields["bucket"]; f == nil || !f.Required || f.CleanDescription != "The name of the bucket." {
			t.Errorf("bucket = %+v", f)
		}
		if f := d.Fields["notification_id"]; f == nil || !f.Output || f.CleanDescription != "The ID of the created notification." {
			t.Errorf("notification_id = %+v", f)
		}
	})

	// Spot-check 4: google_storage_bucket_iam
	t.Run("spotcheck_storage_bucket_iam", func(t *testing.T) {
		d := docs1["google_storage_bucket_iam"]
		if d == nil {
			t.Fatal("missing google_storage_bucket_iam")
		}
		if !d.IsIAM {
			t.Errorf("IsIAM = false, want true")
		}
		if f := d.Fields["bucket"]; f == nil || !f.Required || f.CleanDescription != "Used to find the parent resource to bind the IAM policy to" {
			t.Errorf("bucket = %+v", f)
		}
		m1 := d.Fields["member"]
		m2 := d.Fields["members"]
		if m1 == nil || m2 == nil {
			t.Fatalf("expected both member and members in IAM doc, got member=%v members=%v", m1, m2)
		}
		if m1.Name != "member" || m1.Path != "member" || m2.Name != "members" || m2.Path != "members" {
			t.Errorf("unexpected member/members identity: %+v / %+v", m1, m2)
		}
		if !strings.Contains(m1.CleanDescription, "* **allUsers**:") || m1.CleanDescription != m2.CleanDescription {
			t.Errorf("unexpected member CleanDescription: %q", m1.CleanDescription)
		}
		if _, hasRaw := d.Fields["member/members"]; hasRaw {
			t.Errorf("raw member/members key should not remain in Fields")
		}
		if f := d.Fields["condition.expression"]; f == nil || !f.Required {
			t.Errorf("unanchored IAM condition.expression = %+v", f)
		}
	})

	// Spot-check 5: google_tags_tag_binding_collection (.html.markdown.tmpl)
	t.Run("spotcheck_tags_tag_binding_collection", func(t *testing.T) {
		d := docs1["google_tags_tag_binding_collection"]
		if d == nil {
			t.Fatal("missing google_tags_tag_binding_collection")
		}
		if !d.IsTemplate {
			t.Errorf("IsTemplate = false, want true")
		}
		if d.Subcategory != "Tags" {
			t.Errorf("Subcategory = %q, want Tags", d.Subcategory)
		}
		if f := d.Fields["full_resource_name"]; f == nil || !f.Required || !strings.HasPrefix(f.CleanDescription, "The full resource name of the resource") {
			t.Errorf("full_resource_name = %+v", f)
		}
		if f := d.Fields["active_tags"]; f == nil || !f.Output || !strings.Contains(f.CleanDescription, "\nThis includes the tags configured through Terraform") {
			t.Errorf("active_tags = %+v", f)
		}
	})

	// Additional spot-checks on delegated, DCL-unanchored, and zero-prior-desc resources
	t.Run("spotcheck_additional_tricky_resources", func(t *testing.T) {
		if d := docs1["google_compute_instance_from_template"]; d == nil {
			t.Fatal("missing google_compute_instance_from_template")
		} else {
			if f := d.Fields["source_instance_template"]; f == nil || !f.Required || !strings.Contains(f.CleanDescription, "self_link_unique") {
				t.Errorf("source_instance_template = %+v", f)
			}
			if f := d.Fields["zone"]; f == nil || strings.Contains(f.CleanDescription, "Attributes as Blocks") {
				t.Errorf("zone leaked delegation footer: %+v", f)
			}
		}

		if d := docs1["google_dataplex_zone"]; d == nil {
			t.Fatal("missing google_dataplex_zone")
		} else if f := d.Fields["discovery_spec.csv_options.delimiter"]; f == nil || !f.Optional {
			t.Errorf("dataplex_zone discovery_spec.csv_options.delimiter = %+v", f)
		}

		for _, rname := range []string{"google_storage_object_acl", "google_project_service", "google_billing_subaccount"} {
			if d := docs1[rname]; d == nil || len(d.Fields) == 0 {
				t.Errorf("expected fields for %s, got %+v", rname, d)
			}
		}
	})
}

func TestUnanchoredBlocksAndEnumListsPreserved(t *testing.T) {
	src := `---
subcategory: "Cloud Deploy"
description: |-
  Delivery pipeline resource.
---

# google_example_pipeline

Pipeline overview.

## Argument Reference

* ` + "`mode`" + ` - (Optional) Execution mode:
  * ` + "`allow`" + `: allow execution.
  * ` + "`deny()`" + `: deny execution.

* ` + "`serial_pipeline`" + ` - (Optional) Serial pipeline configuration.

The ` + "`serial_pipeline`" + ` block supports:

* ` + "`canary`" + ` - (Optional) Canary strategy.
* ` + "`standard`" + ` - (Optional) Standard strategy.

The ` + "`canary`" + ` block supports:

* ` + "`postdeploy`" + ` - (Optional) Canary postdeploy.

The ` + "`postdeploy`" + ` block supports:

* ` + "`actions`" + ` - (Optional) Canary postdeploy actions.

The ` + "`standard`" + ` block supports:

* ` + "`postdeploy`" + ` - (Optional) Standard postdeploy.

The ` + "`postdeploy`" + ` block supports:

* ` + "`actions`" + ` - (Optional) Standard postdeploy actions.
`
	doc, err := Parse("example_pipeline.html.markdown", []byte(src))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Enum bullets without schema badges must stay inside `mode.CleanDescription`, NOT become `mode.allow` fields!
	if _, bad := doc.Fields["mode.allow"]; bad {
		t.Errorf("enum bullet `allow` was mistakenly parsed as nested schema field mode.allow")
	}
	if f := doc.Fields["mode"]; f == nil || !strings.Contains(f.CleanDescription, "* `allow`: allow execution.") {
		t.Errorf("mode enum list not preserved: %+v", f)
	}

	// Repeated unanchored `postdeploy` blocks must resolve in DFS order under both `canary` and `standard`.
	if f := doc.Fields["serial_pipeline.canary.postdeploy.actions"]; f == nil || f.CleanDescription != "Canary postdeploy actions." {
		t.Errorf("canary postdeploy actions = %+v", f)
	}
	if f := doc.Fields["serial_pipeline.standard.postdeploy.actions"]; f == nil || f.CleanDescription != "Standard postdeploy actions." {
		t.Errorf("standard postdeploy actions = %+v", f)
	}

	// Error handling on empty filename
	if _, err := Parse("", []byte(src)); err == nil {
		t.Errorf("expected error on empty filename")
	}
}

func TestStructurePointerPreservesPrecedingTheNounSentence(t *testing.T) {
	src := `---
subcategory: "Container"
description: |-
  Cluster resource.
---

# google_example_cluster

## Argument Reference

* ` + "`node_config`" + ` - (Optional) The node configuration of the pool. Structure is [documented below](#nested_node_config).
* ` + "`worker_config`" + ` - (Optional) Configuration for the ` + "`WorkerPool`" + `. Structure is [documented below](#nested_worker_config).
* ` + "`custom_block`" + ` - (Optional) Custom settings. The ` + "`custom_block`" + ` structure is [documented below](#nested_custom_block).

<a name="nested_node_config"></a>The ` + "`node_config`" + ` block supports:

* ` + "`machine_type`" + ` - (Optional) Machine type.

<a name="nested_worker_config"></a>The ` + "`worker_config`" + ` block supports:

* ` + "`disk_size_gb`" + ` - (Optional) Disk size.

<a name="nested_custom_block"></a>The ` + "`custom_block`" + ` block supports:

* ` + "`enabled`" + ` - (Optional) Enabled flag.
`
	doc, err := Parse("example_cluster.html.markdown", []byte(src))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if got := doc.Fields["node_config"].CleanDescription; got != "The node configuration of the pool." {
		t.Errorf("node_config CleanDescription = %q, want %q", got, "The node configuration of the pool.")
	}
	if got := doc.Fields["worker_config"].CleanDescription; got != "Configuration for the `WorkerPool`." {
		t.Errorf("worker_config CleanDescription = %q, want %q", got, "Configuration for the `WorkerPool`.")
	}
	if got := doc.Fields["custom_block"].CleanDescription; got != "Custom settings." {
		t.Errorf("custom_block CleanDescription = %q, want %q", got, "Custom settings.")
	}
}

func TestVerifierEdgeCases_RealCorpusSpotchecks(t *testing.T) {
	docsRDir := filepath.Join("..", "..", "..", "mmv1", "third_party", "terraform", "website", "docs", "r")

	// 1. dataproc_cluster: (Input only) badge on node_pool_config + tab/8-space indentation on attached_disk_config.
	dp, err := ParseFile(filepath.Join(docsRDir, "dataproc_cluster.html.markdown"))
	if err != nil {
		t.Fatalf("ParseFile dataproc_cluster: %v", err)
	}
	roles := dp.Fields["virtual_cluster_config.kubernetes_cluster_config.gke_cluster_config.node_pool_target.roles"]
	if roles == nil || strings.Contains(roles.CleanDescription, "node_pool_config") {
		t.Errorf("roles polluted by node_pool_config: %+v", roles)
	}
	for _, wantPath := range []string{
		"virtual_cluster_config.kubernetes_cluster_config.gke_cluster_config.node_pool_target.node_pool_config",
		"virtual_cluster_config.kubernetes_cluster_config.gke_cluster_config.node_pool_target.node_pool_config.autoscaling.min_node_count",
		"virtual_cluster_config.kubernetes_cluster_config.gke_cluster_config.node_pool_target.node_pool_config.config.machine_type",
		"cluster_config.master_config.disk_config.attached_disk_config.disk_size_gb",
		"cluster_config.preemptible_worker_config.disk_config.boot_disk_provisioned_iops",
		"cluster_config.preemptible_worker_config.disk_config.attached_disk_config.disk_size_gb",
	} {
		if f := dp.Fields[wantPath]; f == nil || f.CleanDescription == "" {
			t.Errorf("dataproc_cluster missing %q: %+v", wantPath, f)
		}
	}

	// 2. clouddeploy_delivery_pipeline: shared unanchored verify_config / analysis / tasks across canary & standard.
	cd, err := ParseFile(filepath.Join(docsRDir, "clouddeploy_delivery_pipeline.html.markdown"))
	if err != nil {
		t.Fatalf("ParseFile clouddeploy_delivery_pipeline: %v", err)
	}
	for _, wantPath := range []string{
		"serial_pipeline.stages.strategy.canary.canary_deployment.verify_config.tasks.container.image",
		"serial_pipeline.stages.strategy.canary.canary_deployment.analysis.google_cloud.alert_policy_checks.alert_policies",
		"serial_pipeline.stages.strategy.canary.custom_canary_deployment.phase_configs.verify_config.tasks.container.image",
		"serial_pipeline.stages.strategy.canary.custom_canary_deployment.phase_configs.analysis.custom_checks.task.container.image",
		"serial_pipeline.stages.strategy.standard.verify_config.tasks.container.image",
		"serial_pipeline.stages.strategy.standard.analysis.duration",
	} {
		if f := cd.Fields[wantPath]; f == nil || f.CleanDescription == "" {
			t.Errorf("clouddeploy_delivery_pipeline missing %q: %+v", wantPath, f)
		}
	}

	// 3. composer_environment: unanchored `The `web_server_network_access_control` supports:` (without `block`).
	comp, err := ParseFile(filepath.Join(docsRDir, "composer_environment.html.markdown"))
	if err != nil {
		t.Fatalf("ParseFile composer_environment: %v", err)
	}
	for _, wantPath := range []string{
		"config.web_server_network_access_control.allowed_ip_range",
		"config.web_server_network_access_control.allowed_ip_range.value",
		"config.web_server_network_access_control.allowed_ip_range.description",
	} {
		if f := comp.Fields[wantPath]; f == nil || f.CleanDescription == "" {
			t.Errorf("composer_environment missing %q: %+v", wantPath, f)
		}
	}

	// 4. container_cluster: maintenance_policy.disruption_budget.* + writable_cgroups cleanup.
	cc, err := ParseFile(filepath.Join(docsRDir, "container_cluster.html.markdown"))
	if err != nil {
		t.Fatalf("ParseFile container_cluster: %v", err)
	}
	for _, wantPath := range []string{
		"maintenance_policy.disruption_budget.minor_version_disruption_interval",
		"maintenance_policy.disruption_budget.patch_version_disruption_interval",
		"maintenance_policy.disruption_budget.last_minor_version_disruption_time",
		"maintenance_policy.disruption_budget.last_disruption_time",
	} {
		if f := cc.Fields[wantPath]; f == nil || f.CleanDescription == "" {
			t.Errorf("container_cluster missing %q: %+v", wantPath, f)
		}
	}
	wc := cc.Fields["node_config.containerd_config.writable_cgroups"]
	if wc == nil || strings.Contains(wc.CleanDescription, "block supports:") {
		t.Errorf("container_cluster writable_cgroups leaked block header: %+v", wc)
	}
}
