package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GoogleCloudPlatform/magic-modules/tools/doc-backfill/backfill"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doc-backfill", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		repoRoot       string
		docsDir        string
		servicesDir    string
		serviceCSV     string
		resourceCSV    string
		modeStr        string
		includeIAM     bool
		includeResDesc bool
		writeFiles     bool
		checkOnly      bool
		reportJSON     string
	)

	fs.StringVar(&repoRoot, "repo-root", "", "Path to magic-modules repository root (auto-detected if omitted)")
	fs.StringVar(&docsDir, "docs-dir", "", "Override path to Registry Markdown resource docs (defaults to <repo-root>/mmv1/third_party/terraform/website/docs/r)")
	fs.StringVar(&servicesDir, "services-dir", "", "Override path to handwritten Go services root (defaults to <repo-root>/mmv1/third_party/terraform/services)")
	fs.StringVar(&serviceCSV, "service", "", "Optional comma-separated service directory filter (e.g. compute,storage)")
	fs.StringVar(&resourceCSV, "resource", "", "Optional comma-separated resource name filter (e.g. google_compute_instance)")
	fs.StringVar(&modeStr, "mode", string(backfill.ModeSync), "Backfill mode: 'sync' (populate missing + update stale literals) or 'missing' (only populate missing/empty)")
	fs.BoolVar(&includeIAM, "include-iam", true, "Backfill handwritten IAM parent schema fields from *_iam.html.markdown")
	fs.BoolVar(&includeResDesc, "include-resource-description", true, "Backfill Plugin Framework resp.Schema.Description from Markdown overview/page description")
	fs.BoolVar(&writeFiles, "write", false, "Write patched .go and .go.tmpl files to disk (default: dry-run)")
	fs.BoolVar(&checkOnly, "check", false, "Exit with code 1 if any file has pending backfill diffs (CI enforcement)")
	fs.StringVar(&reportJSON, "report-json", "", "Optional file path to write deterministic JSON report")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected positional arguments: %v\n", fs.Args())
		return 2
	}

	opts := backfill.Options{
		RepoRoot:                   repoRoot,
		DocsDir:                    docsDir,
		ServicesDir:                servicesDir,
		Services:                   backfill.SplitCSV(serviceCSV),
		Resources:                  backfill.SplitCSV(resourceCSV),
		Mode:                       backfill.Mode(modeStr),
		IncludeIAM:                 includeIAM,
		IncludeResourceDescription: includeResDesc,
		Write:                      writeFiles,
		Check:                      checkOnly,
		ReportJSON:                 reportJSON,
	}

	res, err := backfill.Run(opts)
	if err != nil {
		fmt.Fprintf(stderr, "doc-backfill error: %v\n", err)
		return 2
	}

	r := res.Report
	actionLabel := "dry-run"
	if writeFiles {
		actionLabel = "written"
	} else if checkOnly {
		actionLabel = "check"
	}

	fmt.Fprintf(
		stdout,
		"doc-backfill (%s, mode=%s): docs=%d resources_indexed=%d resources_matched=%d fields_populated=%d fields_updated=%d fields_unchanged=%d res_descriptions=%d skipped_non_literal=%d helper_conflicts=%d files_modified=%d total_edits=%d\n",
		actionLabel,
		r.Mode,
		r.DocsParsed,
		r.ResourcesIndexed,
		r.ResourcesMatched,
		r.FieldsMissingPopulated,
		r.FieldsUpdated,
		r.FieldsUnchanged,
		r.ResourceDescriptionsPatched,
		len(r.SkippedNonLiteral),
		len(r.SharedHelperConflicts),
		r.FilesModified,
		r.TotalEditsApplied,
	)

	if checkOnly && res.HasPendingChanges() {
		fmt.Fprintf(stderr, "doc-backfill --check failed: %d file(s) have %d pending documentation backfill edit(s)\n", r.FilesModified, r.TotalEditsApplied)
		for _, mf := range r.ModifiedFiles {
			fmt.Fprintf(stderr, "  - %s (%d edit(s))\n", mf.FilePath, mf.AppliedCount)
		}
		return 1
	}

	return 0
}
