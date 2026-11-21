package backfill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GoogleCloudPlatform/magic-modules/tools/doc-backfill/gopatcher"
	"github.com/GoogleCloudPlatform/magic-modules/tools/doc-backfill/markdown"
)

// Mode controls which schema fields are eligible for backfilling.
type Mode string

const (
	// ModeSync populates missing Descriptions and updates existing literal
	// Descriptions where the Registry Markdown CleanDescription is non-empty and differs.
	ModeSync Mode = "sync"
	// ModeMissing only populates fields where HasDescription is false or CurrentDescription is empty.
	ModeMissing Mode = "missing"
)

// Options configures a deterministic documentation backfill run.
type Options struct {
	RepoRoot                   string
	DocsDir                    string
	ServicesDir                string
	Services                   []string
	Resources                  []string
	Mode                       Mode
	IncludeIAM                 bool
	IncludeResourceDescription bool
	Write                      bool
	Check                      bool
	ReportJSON                 string
}

// SkippedField records a Go schema field whose Description could not be overwritten safely.
type SkippedField struct {
	ResourceName string `json:"resource_name"`
	FieldPath    string `json:"field_path"`
	FilePath     string `json:"file_path"`
	TargetID     string `json:"target_id"`
	SharedHelper string `json:"shared_helper,omitempty"`
	Reason       string `json:"reason"`
}

// SharedHelperConflict records two resources proposing different non-empty descriptions
// for the exact same AST composite literal (TargetID) in a shared *Schema() helper.
type SharedHelperConflict struct {
	TargetID               string `json:"target_id"`
	FilePath               string `json:"file_path"`
	SharedHelper           string `json:"shared_helper,omitempty"`
	ChosenResource         string `json:"chosen_resource"`
	ChosenFieldPath        string `json:"chosen_field_path"`
	ChosenDescription      string `json:"chosen_description"`
	ConflictingResource    string `json:"conflicting_resource"`
	ConflictingFieldPath   string `json:"conflicting_field_path"`
	ConflictingDescription string `json:"conflicting_description"`
}

// ModifiedFileSummary summarizes applied edits per file in sorted path order.
type ModifiedFileSummary struct {
	FilePath     string `json:"file_path"`
	AppliedCount int    `json:"applied_count"`
}

// ResourceReport summarizes backfill metrics for a single matched resource.
type ResourceReport struct {
	ResourceName               string            `json:"resource_name"`
	Service                    string            `json:"service"`
	Kind                       gopatcher.SDKKind `json:"kind"`
	DocPath                    string            `json:"doc_path"`
	PrimaryFile                string            `json:"primary_file"`
	MatchedFields              int               `json:"matched_fields"`
	PopulatedFields            int               `json:"populated_fields"`
	UpdatedFields              int               `json:"updated_fields"`
	UnchangedFields            int               `json:"unchanged_fields"`
	SkippedNonLiteralFields    int               `json:"skipped_non_literal_fields"`
	UnmatchedSchemaFields      int               `json:"unmatched_schema_fields"`
	ResourceDescriptionPatched bool              `json:"resource_description_patched"`
}

// Report is the deterministic, JSON-serializable summary of a backfill run.
type Report struct {
	Mode                        Mode                   `json:"mode"`
	DryRun                      bool                   `json:"dry_run"`
	DocsParsed                  int                    `json:"docs_parsed"`
	ResourcesIndexed            int                    `json:"resources_indexed"`
	ResourcesMatched            int                    `json:"resources_matched"`
	FieldsEvaluated             int                    `json:"fields_evaluated"`
	FieldsMissingPopulated      int                    `json:"fields_missing_populated"`
	FieldsUpdated               int                    `json:"fields_updated"`
	FieldsUnchanged             int                    `json:"fields_unchanged"`
	ResourceDescriptionsPatched int                    `json:"resource_descriptions_patched"`
	FilesModified               int                    `json:"files_modified"`
	TotalEditsApplied           int                    `json:"total_edits_applied"`
	ResourcesWithoutDocs        []string               `json:"resources_without_docs"`
	DocsWithoutResources        []string               `json:"docs_without_resources"`
	SkippedNonLiteral           []SkippedField         `json:"skipped_non_literal"`
	SharedHelperConflicts       []SharedHelperConflict `json:"shared_helper_conflicts"`
	ModifiedFiles               []ModifiedFileSummary  `json:"modified_files"`
	Resources                   []ResourceReport       `json:"resources"`
}

// Result holds both the Report and raw FileDiffs produced by Run.
type Result struct {
	Report    Report
	FileDiffs []gopatcher.FileDiff
}

// HasPendingChanges reports whether any file has pending backfill edits.
func (r *Result) HasPendingChanges() bool {
	return r != nil && len(r.FileDiffs) > 0
}

// docReference mirrors tools/diff-processor/detector/detector.go cross-resource
// Markdown documentation delegations.
type docReference struct {
	Prefix    string
	Resource  string
	DocPrefix string
}

var resourceDocReferences = map[string][]docReference{
	"google_compute_instance_from_machine_image": {{Resource: "google_compute_instance"}},
	"google_compute_instance_from_template":      {{Resource: "google_compute_instance"}},
	"google_container_cluster":                   {{Prefix: "node_pool.", Resource: "google_container_node_pool"}},
	"google_container_node_pool":                 {{Prefix: "node_config.", Resource: "google_container_cluster", DocPrefix: "node_config."}},
}

const maxDocReferenceDepth = 2

// SplitCSV splits a comma-separated flag value into trimmed, non-empty unique tokens.
func SplitCSV(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(csv, ",") {
		tok := strings.TrimSpace(part)
		if tok == "" || seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	sort.Strings(out)
	return out
}

// DetectRepoRoot locates the magic-modules root directory by walking upward
// from the current working directory and (as fallback) the executable path.
func DetectRepoRoot() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil && wd != "" {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		if root, ok := walkUpForRepoRoot(start); ok {
			return root, nil
		}
	}
	return "", errors.New("could not auto-detect magic-modules repository root; pass --repo-root or both --docs-dir and --services-dir")
}

func walkUpForRepoRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		docsCheck := filepath.Join(dir, "mmv1", "third_party", "terraform", "website", "docs", "r")
		svcsCheck := filepath.Join(dir, "mmv1", "third_party", "terraform", "services")
		if isDir(docsCheck) && isDir(svcsCheck) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

type resolvedFieldDoc struct {
	FieldDoc       *markdown.FieldDoc
	SourceResource string
	Delegated      bool
}

type candidateProposal struct {
	ResourceName string
	IsIAM        bool
	Delegated    bool
	FieldPath    string
	Target       *gopatcher.FieldTarget
	CleanDesc    string
}

func (c candidateProposal) tier() int {
	if c.Delegated {
		return 2
	}
	if c.IsIAM {
		return 1
	}
	return 0
}

// Run executes the deterministic Registry Markdown -> Go schema backfill pipeline.
func Run(opts Options) (*Result, error) {
	if opts.Mode == "" {
		opts.Mode = ModeSync
	}
	if opts.Mode != ModeSync && opts.Mode != ModeMissing {
		return nil, fmt.Errorf("invalid --mode %q: must be %q or %q", opts.Mode, ModeSync, ModeMissing)
	}
	if opts.Check && opts.Write {
		return nil, errors.New("--check and --write cannot be used together")
	}

	repoRoot := strings.TrimSpace(opts.RepoRoot)
	docsDir := strings.TrimSpace(opts.DocsDir)
	servicesDir := strings.TrimSpace(opts.ServicesDir)

	if docsDir == "" || servicesDir == "" {
		if repoRoot == "" {
			detected, err := DetectRepoRoot()
			if err != nil {
				return nil, err
			}
			repoRoot = detected
		} else {
			absRoot, err := filepath.Abs(repoRoot)
			if err != nil {
				return nil, fmt.Errorf("resolve --repo-root: %w", err)
			}
			repoRoot = absRoot
		}
		if docsDir == "" {
			docsDir = filepath.Join(repoRoot, "mmv1", "third_party", "terraform", "website", "docs", "r")
		}
		if servicesDir == "" {
			servicesDir = filepath.Join(repoRoot, "mmv1", "third_party", "terraform", "services")
		}
	} else if repoRoot != "" {
		if absRoot, err := filepath.Abs(repoRoot); err == nil {
			repoRoot = absRoot
		}
	}

	docsDir, err := filepath.Abs(docsDir)
	if err != nil {
		return nil, fmt.Errorf("resolve docs-dir: %w", err)
	}
	servicesDir, err = filepath.Abs(servicesDir)
	if err != nil {
		return nil, fmt.Errorf("resolve services-dir: %w", err)
	}

	// 1. Parse all Registry Markdown resource docs so cross-resource delegations always work.
	docsByRes, err := markdown.ParseDir(docsDir)
	if err != nil {
		return nil, fmt.Errorf("parse markdown docs in %s: %w", docsDir, err)
	}

	// 2. Index handwritten Go/template resource schemas (scoped to --service if provided).
	serviceFilter := normalizeTokenSet(opts.Services)
	resourceFilter := normalizeTokenSet(opts.Resources)

	targetsByRes := make(map[string]*gopatcher.ResourceTarget)
	if len(serviceFilter) > 0 {
		sortedServices := sortedKeys(serviceFilter)
		for _, svc := range sortedServices {
			svcPath := filepath.Join(servicesDir, svc)
			if !isDir(svcPath) {
				return nil, fmt.Errorf("service directory %q does not exist under %s", svc, servicesDir)
			}
			svcTargets, err := gopatcher.IndexServiceDir(svcPath)
			if err != nil {
				return nil, fmt.Errorf("index service %s: %w", svc, err)
			}
			for _, rt := range svcTargets {
				targetsByRes[rt.ResourceName] = rt
			}
		}
	} else {
		targetsByRes, err = gopatcher.IndexServicesRoot(servicesDir)
		if err != nil {
			return nil, fmt.Errorf("index services in %s: %w", servicesDir, err)
		}
	}

	// 3. Filter resources deterministically in sorted ResourceName order.
	allIndexedNames := make([]string, 0, len(targetsByRes))
	for name := range targetsByRes {
		allIndexedNames = append(allIndexedNames, name)
	}
	sort.Strings(allIndexedNames)

	var selectedNames []string
	for _, name := range allIndexedNames {
		rt := targetsByRes[name]
		if !opts.IncludeIAM && rt.Kind == gopatcher.IAM {
			continue
		}
		if len(serviceFilter) > 0 && !serviceFilter[rt.Service] {
			continue
		}
		if len(resourceFilter) > 0 && !matchesResourceFilter(rt, resourceFilter) {
			continue
		}
		selectedNames = append(selectedNames, name)
	}

	// Compute DocsWithoutResources (scoped appropriately when unfiltered).
	docsWithoutResources := make([]string, 0)
	if len(serviceFilter) == 0 && len(resourceFilter) == 0 {
		allDocNames := make([]string, 0, len(docsByRes))
		for name := range docsByRes {
			allDocNames = append(allDocNames, name)
		}
		sort.Strings(allDocNames)
		for _, docName := range allDocNames {
			d := docsByRes[docName]
			if !opts.IncludeIAM && d.IsIAM {
				continue
			}
			if _, ok := targetsByRes[docName]; !ok {
				docsWithoutResources = append(docsWithoutResources, docName)
			}
		}
	}

	resourcesWithoutDocs := make([]string, 0)
	resReportsByName := make(map[string]*ResourceReport, len(selectedNames))
	var orderedMatchedNames []string
	var resPatches []gopatcher.ResourceDescPatch
	var skippedNonLiteral []SkippedField
	seenSkipped := make(map[string]bool)

	proposalsByTargetID := make(map[string][]candidateProposal)

	for _, resName := range selectedNames {
		rt := targetsByRes[resName]
		doc, ok := docsByRes[resName]
		if !ok || doc == nil {
			resourcesWithoutDocs = append(resourcesWithoutDocs, resName)
			continue
		}

		orderedMatchedNames = append(orderedMatchedNames, resName)
		rr := &ResourceReport{
			ResourceName: resName,
			Service:      rt.Service,
			Kind:         rt.Kind,
			DocPath:      displayPath(repoRoot, doc.DocPath),
			PrimaryFile:  displayPath(repoRoot, rt.PrimaryFile),
		}
		resReportsByName[resName] = rr

		// Optional Plugin Framework resource-level Schema.Description backfill.
		if opts.IncludeResourceDescription && rt.CanPatchResDesc {
			desiredResDesc := strings.TrimSpace(doc.Overview)
			if desiredResDesc == "" {
				desiredResDesc = strings.TrimSpace(doc.PageDescription)
			}
			if desiredResDesc != "" {
				shouldPatchRes := false
				switch opts.Mode {
				case ModeMissing:
					shouldPatchRes = !rt.HasResDescription || strings.TrimSpace(rt.CurrentResDesc) == ""
				case ModeSync:
					shouldPatchRes = !rt.HasResDescription || rt.CurrentResDesc != desiredResDesc
				}
				if shouldPatchRes {
					resPatches = append(resPatches, gopatcher.ResourceDescPatch{
						Resource:       rt,
						NewDescription: desiredResDesc,
					})
					rr.ResourceDescriptionPatched = true
				}
			}
		}

		effectiveDocs := resolveEffectiveFieldDocs(resName, docsByRes, 0)

		schemaPaths := make([]string, 0, len(rt.Fields))
		for fp := range rt.Fields {
			schemaPaths = append(schemaPaths, fp)
		}
		sort.Strings(schemaPaths)

		for _, fp := range schemaPaths {
			targets := append([]*gopatcher.FieldTarget(nil), rt.Fields[fp]...)
			if len(targets) == 0 {
				continue
			}
			sort.Slice(targets, func(i, j int) bool {
				return targets[i].TargetID < targets[j].TargetID
			})

			rfd, hasDoc := effectiveDocs[fp]
			if !hasDoc || rfd.FieldDoc == nil || strings.TrimSpace(rfd.FieldDoc.CleanDescription) == "" {
				// Never overwrite a non-empty Go Description with an empty CleanDescription,
				// and skip implicit/undocumented root "id" or "timeouts" noise in unmatched counts.
				if fp != "id" && fp != "timeouts" && !strings.HasPrefix(fp, "timeouts.") {
					rr.UnmatchedSchemaFields++
				}
				continue
			}

			cleanDesc := rfd.FieldDoc.CleanDescription
			rr.MatchedFields++

			for _, ft := range targets {
				if ft == nil {
					continue
				}
				if !ft.IsLiteralOrMissing {
					rr.SkippedNonLiteralFields++
					skipKey := resName + "|" + fp + "|" + ft.TargetID
					if !seenSkipped[skipKey] {
						seenSkipped[skipKey] = true
						skippedNonLiteral = append(skippedNonLiteral, SkippedField{
							ResourceName: resName,
							FieldPath:    fp,
							FilePath:     displayPath(repoRoot, ft.FilePath),
							TargetID:     displayTargetID(repoRoot, ft.TargetID),
							SharedHelper: ft.SharedHelper,
							Reason:       "non-literal description expression",
						})
					}
					continue
				}

				proposalsByTargetID[ft.TargetID] = append(proposalsByTargetID[ft.TargetID], candidateProposal{
					ResourceName: resName,
					IsIAM:        rt.Kind == gopatcher.IAM || doc.IsIAM,
					Delegated:    rfd.Delegated,
					FieldPath:    fp,
					Target:       ft,
					CleanDesc:    cleanDesc,
				})
			}
		}
	}

	// 4. Deterministically deduplicate shared *Schema() helper targets by TargetID.
	sortedTargetIDs := make([]string, 0, len(proposalsByTargetID))
	for tid := range proposalsByTargetID {
		sortedTargetIDs = append(sortedTargetIDs, tid)
	}
	sort.Strings(sortedTargetIDs)

	var patches []gopatcher.PatchRequest
	var conflicts []SharedHelperConflict
	fieldsEvaluated := 0
	fieldsPopulated := 0
	fieldsUpdated := 0
	fieldsUnchanged := 0

	for _, tid := range sortedTargetIDs {
		cands := proposalsByTargetID[tid]
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].tier() != cands[j].tier() {
				return cands[i].tier() < cands[j].tier()
			}
			if cands[i].ResourceName != cands[j].ResourceName {
				return cands[i].ResourceName < cands[j].ResourceName
			}
			return cands[i].FieldPath < cands[j].FieldPath
		})

		winner := cands[0]
		seenConflictKey := make(map[string]bool)
		for _, other := range cands[1:] {
			if other.CleanDesc == winner.CleanDesc {
				continue
			}
			ckey := other.ResourceName + "|" + other.FieldPath + "|" + other.CleanDesc
			if seenConflictKey[ckey] {
				continue
			}
			seenConflictKey[ckey] = true
			conflicts = append(conflicts, SharedHelperConflict{
				TargetID:               displayTargetID(repoRoot, tid),
				FilePath:               displayPath(repoRoot, winner.Target.FilePath),
				SharedHelper:           winner.Target.SharedHelper,
				ChosenResource:         winner.ResourceName,
				ChosenFieldPath:        winner.FieldPath,
				ChosenDescription:      winner.CleanDesc,
				ConflictingResource:    other.ResourceName,
				ConflictingFieldPath:   other.FieldPath,
				ConflictingDescription: other.CleanDesc,
			})
		}

		fieldsEvaluated++
		winnerRR := resReportsByName[winner.ResourceName]
		isMissing := !winner.Target.HasDescription || winner.Target.CurrentDescription == ""

		switch opts.Mode {
		case ModeMissing:
			if isMissing {
				patches = append(patches, gopatcher.PatchRequest{
					Target:         winner.Target,
					NewDescription: winner.CleanDesc,
				})
				fieldsPopulated++
				winnerRR.PopulatedFields++
			} else {
				fieldsUnchanged++
				winnerRR.UnchangedFields++
			}
		case ModeSync:
			if isMissing {
				patches = append(patches, gopatcher.PatchRequest{
					Target:         winner.Target,
					NewDescription: winner.CleanDesc,
				})
				fieldsPopulated++
				winnerRR.PopulatedFields++
			} else if winner.Target.CurrentDescription != winner.CleanDesc {
				patches = append(patches, gopatcher.PatchRequest{
					Target:         winner.Target,
					NewDescription: winner.CleanDesc,
				})
				fieldsUpdated++
				winnerRR.UpdatedFields++
			} else {
				fieldsUnchanged++
				winnerRR.UnchangedFields++
			}
		}
	}

	sort.Slice(patches, func(i, j int) bool {
		if patches[i].Target.FilePath != patches[j].Target.FilePath {
			return patches[i].Target.FilePath < patches[j].Target.FilePath
		}
		return patches[i].Target.TargetID < patches[j].Target.TargetID
	})
	sort.Slice(resPatches, func(i, j int) bool {
		if resPatches[i].Resource.PrimaryFile != resPatches[j].Resource.PrimaryFile {
			return resPatches[i].Resource.PrimaryFile < resPatches[j].Resource.PrimaryFile
		}
		return resPatches[i].Resource.ResourceName < resPatches[j].Resource.ResourceName
	})

	// 5. Apply surgical byte-span patches (in-memory first; write only when opts.Write is true).
	var activeDiffs []gopatcher.FileDiff
	if len(patches) > 0 || len(resPatches) > 0 {
		rawDiffs, err := gopatcher.ApplyPatches(patches, resPatches)
		if err != nil {
			return nil, fmt.Errorf("apply patches: %w", err)
		}
		for _, d := range rawDiffs {
			if d.AppliedCount > 0 && !bytes.Equal(d.OriginalBytes, d.UpdatedBytes) {
				activeDiffs = append(activeDiffs, d)
			}
		}
		sort.Slice(activeDiffs, func(i, j int) bool {
			return activeDiffs[i].FilePath < activeDiffs[j].FilePath
		})
	}

	if opts.Write {
		for _, d := range activeDiffs {
			perm := os.FileMode(0644)
			if st, err := os.Stat(d.FilePath); err == nil {
				perm = st.Mode().Perm()
			}
			if err := os.WriteFile(d.FilePath, d.UpdatedBytes, perm); err != nil {
				return nil, fmt.Errorf("write %s: %w", d.FilePath, err)
			}
		}
	}

	// 6. Assemble deterministic report slices.
	sort.Slice(skippedNonLiteral, func(i, j int) bool {
		if skippedNonLiteral[i].ResourceName != skippedNonLiteral[j].ResourceName {
			return skippedNonLiteral[i].ResourceName < skippedNonLiteral[j].ResourceName
		}
		if skippedNonLiteral[i].FieldPath != skippedNonLiteral[j].FieldPath {
			return skippedNonLiteral[i].FieldPath < skippedNonLiteral[j].FieldPath
		}
		return skippedNonLiteral[i].TargetID < skippedNonLiteral[j].TargetID
	})
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].TargetID != conflicts[j].TargetID {
			return conflicts[i].TargetID < conflicts[j].TargetID
		}
		if conflicts[i].ConflictingResource != conflicts[j].ConflictingResource {
			return conflicts[i].ConflictingResource < conflicts[j].ConflictingResource
		}
		return conflicts[i].ConflictingFieldPath < conflicts[j].ConflictingFieldPath
	})

	modifiedSummaries := make([]ModifiedFileSummary, 0, len(activeDiffs))
	totalEdits := 0
	for _, d := range activeDiffs {
		modifiedSummaries = append(modifiedSummaries, ModifiedFileSummary{
			FilePath:     displayPath(repoRoot, d.FilePath),
			AppliedCount: d.AppliedCount,
		})
		totalEdits += d.AppliedCount
	}

	orderedReports := make([]ResourceReport, 0, len(orderedMatchedNames))
	for _, name := range orderedMatchedNames {
		orderedReports = append(orderedReports, *resReportsByName[name])
	}

	if skippedNonLiteral == nil {
		skippedNonLiteral = []SkippedField{}
	}
	if conflicts == nil {
		conflicts = []SharedHelperConflict{}
	}

	report := Report{
		Mode:                        opts.Mode,
		DryRun:                      !opts.Write,
		DocsParsed:                  len(docsByRes),
		ResourcesIndexed:            len(targetsByRes),
		ResourcesMatched:            len(orderedMatchedNames),
		FieldsEvaluated:             fieldsEvaluated + len(skippedNonLiteral),
		FieldsMissingPopulated:      fieldsPopulated,
		FieldsUpdated:               fieldsUpdated,
		FieldsUnchanged:             fieldsUnchanged,
		ResourceDescriptionsPatched: len(resPatches),
		FilesModified:               len(activeDiffs),
		TotalEditsApplied:           totalEdits,
		ResourcesWithoutDocs:        resourcesWithoutDocs,
		DocsWithoutResources:        docsWithoutResources,
		SkippedNonLiteral:           skippedNonLiteral,
		SharedHelperConflicts:       conflicts,
		ModifiedFiles:               modifiedSummaries,
		Resources:                   orderedReports,
	}

	if opts.ReportJSON != "" {
		if err := WriteReportJSON(opts.ReportJSON, report); err != nil {
			return nil, err
		}
	}

	return &Result{
		Report:    report,
		FileDiffs: activeDiffs,
	}, nil
}

// WriteReportJSON writes the report as deterministically formatted JSON.
func WriteReportJSON(path string, report Report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report json: %w", err)
	}
	data = append(data, '\n')
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create report directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write report json %s: %w", path, err)
	}
	return nil
}

func resolveEffectiveFieldDocs(resName string, docsByRes map[string]*markdown.ResourceDoc, depth int) map[string]resolvedFieldDoc {
	out := make(map[string]resolvedFieldDoc)
	if doc, ok := docsByRes[resName]; ok && doc != nil {
		paths := make([]string, 0, len(doc.Fields))
		for p := range doc.Fields {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			fd := doc.Fields[p]
			if fd == nil || strings.TrimSpace(fd.CleanDescription) == "" {
				continue
			}
			out[p] = resolvedFieldDoc{
				FieldDoc:       fd,
				SourceResource: resName,
				Delegated:      false,
			}
		}
	}

	if refs, ok := resourceDocReferences[resName]; ok && depth < maxDocReferenceDepth {
		for _, ref := range refs {
			refDocs := resolveEffectiveFieldDocs(ref.Resource, docsByRes, depth+1)
			refPaths := make([]string, 0, len(refDocs))
			for p := range refDocs {
				refPaths = append(refPaths, p)
			}
			sort.Strings(refPaths)
			for _, refPath := range refPaths {
				var mappedPath string
				if ref.DocPrefix == "" {
					mappedPath = ref.Prefix + refPath
				} else if suffix, ok := strings.CutPrefix(refPath, ref.DocPrefix); ok {
					mappedPath = ref.Prefix + suffix
				} else {
					continue
				}
				if _, exists := out[mappedPath]; !exists {
					delegated := refDocs[refPath]
					delegated.Delegated = true
					out[mappedPath] = delegated
				}
			}
		}
	}

	return out
}

func matchesResourceFilter(rt *gopatcher.ResourceTarget, filter map[string]bool) bool {
	if filter[rt.ResourceName] {
		return true
	}
	if rt.Kind == gopatcher.IAM {
		for _, suffix := range []string{"_binding", "_member", "_policy"} {
			if filter[rt.ResourceName+suffix] {
				return true
			}
		}
	}
	return false
}

func normalizeTokenSet(items []string) map[string]bool {
	out := make(map[string]bool)
	for _, raw := range items {
		for _, part := range strings.Split(raw, ",") {
			tok := strings.TrimSpace(part)
			if tok != "" {
				out[tok] = true
			}
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func displayPath(repoRoot, p string) string {
	if repoRoot != "" {
		if rel, err := filepath.Rel(repoRoot, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(p)
}

func displayTargetID(repoRoot, targetID string) string {
	idx := strings.LastIndex(targetID, ":")
	if idx <= 0 {
		return targetID
	}
	return displayPath(repoRoot, targetID[:idx]) + targetID[idx:]
}
