# `doc-backfill`

`doc-backfill` is a deterministic, zero-dependency Go tool that backfills Registry Markdown documentation (`mmv1/third_party/terraform/website/docs/r/*`) into handwritten Terraform Provider Google (`TPG` / `TPGB`) Go resource schemas (`mmv1/third_party/terraform/services/**`).

It supports:
- **SDKv2 handwritten resources** (`.go` and `.go.tmpl` templates, nested `&schema.Resource`, package-level `*Schema()` helper functions, schema map variables, and cloned schema overrides such as `google_compute_instance_from_template`)
- **Terraform Plugin Framework resources** (`registry.FrameworkResource`, recursive `Attributes` and `Blocks`, plus resource-level `resp.Schema.Description`)
- **Handwritten IAM parent schemas** (`registry.SchemaTypeIAMResource` + `*_iam.html.markdown`)

## Usage

Run from `tools/doc-backfill/` (or pass `--repo-root`):

```bash
# Dry-run across all handwritten resources and write a JSON summary report
go run . --report-json /tmp/doc-backfill-report.json

# Populate only missing/empty Description fields (do not update existing descriptions)
go run . --mode missing --write

# Full sync (populate missing + update stale literal descriptions) scoped to specific services or resources
go run . --mode sync --service compute,storage --write
go run . --mode sync --resource google_compute_instance --write

# CI / pre-submit check (exits with status 1 if any file has pending backfill diffs)
go run . --check
```

## CLI Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--repo-root <dir>` | auto-detected | Path to `magic-modules` repository root (auto-detected from working directory or binary path when omitted). |
| `--docs-dir <dir>` | `<repo-root>/mmv1/third_party/terraform/website/docs/r` | Override path to Registry Markdown resource documentation files. |
| `--services-dir <dir>` | `<repo-root>/mmv1/third_party/terraform/services` | Override path to handwritten Go service packages. |
| `--service <csv>` | *(all)* | Comma-separated service directory filter (e.g. `compute,storage`). |
| `--resource <csv>` | *(all)* | Comma-separated resource name filter (e.g. `google_compute_instance`). |
| `--mode <sync\|missing>` | `sync` | `sync` populates missing `Description`s and updates stale literal `Description`s; `missing` only populates missing/empty `Description`s. |
| `--include-iam` | `true` | Also backfill handwritten IAM parent schema fields from `*_iam.html.markdown`. |
| `--include-resource-description` | `true` | Also backfill Plugin Framework `resp.Schema.Description` from Markdown overview / page description. |
| `--write` | `false` | Write patched `.go` and `.go.tmpl` files to disk (defaults to read-only dry-run). |
| `--check` | `false` | Exit with code `1` if any file has pending backfill diffs. |
| `--report-json <file>` | *(none)* | Write a deterministic JSON report with per-resource metrics, skipped non-literal fields, and shared helper conflict resolutions. |

## Architecture

- **`markdown/`**: Parses Registry Markdown resource docs (`.html.markdown` and `.html.markdown.tmpl`), resolving nested `<a name="nested_...">` blocks via BFS, normalizing `.0.` attribute paths, expanding IAM `member/members`, and stripping doc-only badges (`(Required)`, `(Optional)`, `[Beta](...)`) and structural `Structure is [documented below]` pointers while preserving multi-paragraph prose, callouts, links, and code blocks.
- **`gopatcher/`**: Masks Go `.go.tmpl` directives with 1:1 byte-offset fidelity so `go/parser` indexes 100% of `.go` and `.go.tmpl` service files, discovers SDKv2, Plugin Framework, and IAM schema composite literals, and applies surgical byte-span edits that preserve `.go.tmpl` guards and comments untouched.
- **`backfill/`**: Deterministic matching engine that resolves cross-resource doc delegations (`google_container_cluster` <-> `google_container_node_pool`, `google_compute_instance_from_template` -> `google_compute_instance`), deduplicates shared `*Schema()` helper targets by `TargetID`, skips non-literal Go expressions safely, and enforces idempotency.
