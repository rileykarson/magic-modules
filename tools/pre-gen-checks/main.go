// Command pre-gen-checks runs fast Phase 1 pre-generation static checks and unit
// tests against magic-modules in parallel with in-memory stream capture.
//
// Checks:
//  1. Submodule check (disallows unauthorized git submodules)
//  2. Go formatting check (gofmt on all tracked .go files)
//  3. Template validation (version-guard and unused-tmpl on modified .tmpl files)
//  4. YAML lint check on changed mmv1/products/ YAML files (if yamllint is installed)
//  5. MMv1 core compiler unit tests (go test ./... in mmv1)
//  6. Internal tools and CI unit tests (diff-scoped to touched packages, or all with -all)
//
// Usage:
//
//	go run ./tools/pre-gen-checks
//	go run ./tools/pre-gen-checks -all
//	go run ./tools/pre-gen-checks -json
//	go run ./tools/pre-gen-checks -base-ref=upstream/main
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type CheckResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "passed", "failed", "skipped"
	DurationMs int64  `json:"duration_ms"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
	Error      string `json:"error,omitempty"`
	Logs       string `json:"logs,omitempty"`
}

type SummaryReport struct {
	Passed     bool          `json:"passed"`
	BaseRef    string        `json:"base_ref"`
	RepoRoot   string        `json:"repo_root"`
	DurationMs int64         `json:"duration_ms"`
	Checks     []CheckResult `json:"checks"`
}

func resolveRepoRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err == nil {
		root := strings.TrimSpace(string(out))
		if info, err := os.Stat(filepath.Join(root, "mmv1")); err == nil && info.IsDir() {
			return root, nil
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(filepath.Join(cwd, "mmv1")); err == nil && info.IsDir() {
		return cwd, nil
	}
	return "", fmt.Errorf("could not locate magic-modules repository root (mmv1 directory missing)")
}

func resolveBaseRef(repoRoot, preferred string) string {
	if preferred != "" {
		if err := exec.Command("git", "-C", repoRoot, "rev-parse", "--verify", preferred).Run(); err == nil {
			return preferred
		}
	}
	// Prefer upstream/main if present, otherwise origin/main, then HEAD
	for _, ref := range []string{"upstream/main", "origin/main", "HEAD"} {
		if err := exec.Command("git", "-C", repoRoot, "rev-parse", "--verify", ref).Run(); err == nil {
			return ref
		}
	}
	return "HEAD"
}

func getChangedFiles(repoRoot, baseRef string) []string {
	// Use --diff-filter=d to exclude deleted files
	cmd := exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=d", baseRef)
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=d", "HEAD")
		out, err = cmd.Output()
		if err != nil {
			return nil
		}
	}
	var files []string
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			files = append(files, trimmed)
		}
	}
	return files
}

func runCommand(ctx context.Context, dir string, cmdName string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func hasPathPrefix(files []string, prefix string) bool {
	for _, f := range files {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

func filterFiles(files []string, predicate func(string) bool) []string {
	var out []string
	for _, f := range files {
		if predicate(f) {
			out = append(out, f)
		}
	}
	return out
}

func main() {
	allFlag := flag.Bool("all", false, "Run unit tests for all internal tools and CI unconditionally")
	jsonFlag := flag.Bool("json", false, "Emit machine-readable JSON output")
	verboseFlag := flag.Bool("v", false, "Verbose output: print check logs even on success")
	baseRefFlag := flag.String("base-ref", "", "Base git ref to diff against (defaults to origin/main or upstream/main)")
	timeoutFlag := flag.Duration("timeout", 5*time.Minute, "Global timeout for all pre-generation checks")
	flag.Parse()

	t0 := time.Now()
	repoRoot, err := resolveRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	baseRef := resolveBaseRef(repoRoot, *baseRefFlag)
	changedFiles := getChangedFiles(repoRoot, baseRef)

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []CheckResult

	addResult := func(r CheckResult) {
		mu.Lock()
		defer mu.Unlock()
		results = append(results, r)
	}

	if !*jsonFlag {
		fmt.Printf("=== Pre-Generation & Static Checks (Go) ===\n")
		fmt.Printf("Repository root: %s\n", repoRoot)
		fmt.Printf("Base ref: %s (%d modified files detected)\n", baseRef, len(changedFiles))
		fmt.Println("Running checks in parallel...")
	}

	// 1. Submodule Check
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		out, err := runCommand(ctx, repoRoot, "git", "submodule", "status", "--recursive")
		dur := time.Since(start).Milliseconds()
		if err != nil || strings.TrimSpace(out) != "" {
			msg := strings.TrimSpace(out)
			if msg == "" {
				msg = fmt.Sprintf("git submodule command failed: %v", err)
			} else {
				msg = fmt.Sprintf("Unauthorized submodules detected:\n%s", msg)
			}
			addResult(CheckResult{
				Name:       "submodule",
				Status:     "failed",
				DurationMs: dur,
				Error:      msg,
				Logs:       msg,
			})
			return
		}
		addResult(CheckResult{
			Name:       "submodule",
			Status:     "passed",
			DurationMs: dur,
			Logs:       "No submodules found. Submodule check passed.",
		})
	}()

	// 2. Go Formatting Check (gofmt)
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		filesCmd := exec.CommandContext(ctx, "git", "ls-files", "*.go")
		filesCmd.Dir = repoRoot
		filesOut, err := filesCmd.Output()
		dur := time.Since(start).Milliseconds()
		if err != nil {
			addResult(CheckResult{
				Name:       "gofmt",
				Status:     "failed",
				DurationMs: dur,
				Error:      fmt.Sprintf("git ls-files failed: %v", err),
			})
			return
		}
		goFiles := strings.Fields(string(filesOut))
		if len(goFiles) == 0 {
			addResult(CheckResult{
				Name:       "gofmt",
				Status:     "passed",
				DurationMs: dur,
				Logs:       "No Go files found.",
			})
			return
		}
		gofmtArgs := append([]string{"-l"}, goFiles...)
		out, err := runCommand(ctx, repoRoot, "gofmt", gofmtArgs...)
		dur = time.Since(start).Milliseconds()
		unformatted := strings.TrimSpace(out)
		if err != nil || unformatted != "" {
			addResult(CheckResult{
				Name:       "gofmt",
				Status:     "failed",
				DurationMs: dur,
				Error:      "Unformatted Go files detected",
				Logs:       fmt.Sprintf("The following files are not formatted properly (run gofmt -w):\n%s", unformatted),
			})
			return
		}
		addResult(CheckResult{
			Name:       "gofmt",
			Status:     "passed",
			DurationMs: dur,
			Logs:       "All tracked Go files are properly formatted.",
		})
	}()

	// 3. Template Validation (tools/template-check)
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		changedTmpls := filterFiles(changedFiles, func(p string) bool {
			if !strings.HasSuffix(p, ".tmpl") {
				return false
			}
			info, err := os.Stat(filepath.Join(repoRoot, p))
			return err == nil && !info.IsDir()
		})
		if len(changedTmpls) == 0 {
			addResult(CheckResult{
				Name:       "template_validation",
				Status:     "passed",
				DurationMs: time.Since(start).Milliseconds(),
				Logs:       "No modified template (.tmpl) files detected.",
			})
			return
		}

		var relTmpls []string
		for _, f := range changedTmpls {
			relTmpls = append(relTmpls, filepath.Join("../..", f))
		}
		fileList := strings.Join(relTmpls, ",")
		toolDir := filepath.Join(repoRoot, "tools", "template-check")

		var logBuf strings.Builder
		out, err := runCommand(ctx, toolDir, "go", "run", "main.go", "version-guard", "--file-list", fileList)
		logBuf.WriteString(out)
		if err != nil {
			addResult(CheckResult{
				Name:       "template_validation",
				Status:     "failed",
				DurationMs: time.Since(start).Milliseconds(),
				Error:      "version-guard check failed",
				Logs:       logBuf.String(),
			})
			return
		}

		// Check for newly added templates relative to base ref
		newTmplCmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "diff", "--name-only", "--diff-filter=A", baseRef, "HEAD", "--", "mmv1")
		if newTmplOut, err := newTmplCmd.Output(); err == nil {
			var newTmpls []string
			for _, line := range strings.Split(string(newTmplOut), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasSuffix(line, ".tmpl") {
					newTmpls = append(newTmpls, filepath.Join("../..", line))
				}
			}
			if len(newTmpls) > 0 {
				newFileList := strings.Join(newTmpls, ",")
				out2, err := runCommand(ctx, toolDir, "go", "run", "main.go", "unused-tmpl", "--file-list", newFileList)
				logBuf.WriteString(out2)
				if err != nil {
					addResult(CheckResult{
						Name:       "template_validation",
						Status:     "failed",
						DurationMs: time.Since(start).Milliseconds(),
						Error:      "unused-tmpl check failed",
						Logs:       logBuf.String(),
					})
					return
				}
			}
		}

		addResult(CheckResult{
			Name:       "template_validation",
			Status:     "passed",
			DurationMs: time.Since(start).Milliseconds(),
			Logs:       logBuf.String(),
		})
	}()

	// 4. YAML Linting Check (yamllint)
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		if _, err := exec.LookPath("yamllint"); err != nil {
			addResult(CheckResult{
				Name:       "yamllint",
				Status:     "skipped",
				DurationMs: time.Since(start).Milliseconds(),
				Skipped:    true,
				SkipReason: "yamllint not found in PATH",
			})
			return
		}
		productYAMLs := filterFiles(changedFiles, func(p string) bool {
			return strings.HasPrefix(p, "mmv1/products/") && (strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml"))
		})
		if len(productYAMLs) == 0 {
			addResult(CheckResult{
				Name:       "yamllint",
				Status:     "passed",
				DurationMs: time.Since(start).Milliseconds(),
				Logs:       "No modified product YAML files to lint.",
			})
			return
		}
		args := append([]string{"-c", ".yamllint"}, productYAMLs...)
		out, err := runCommand(ctx, repoRoot, "yamllint", args...)
		dur := time.Since(start).Milliseconds()
		if err != nil {
			addResult(CheckResult{
				Name:       "yamllint",
				Status:     "failed",
				DurationMs: dur,
				Error:      "yamllint syntax errors detected",
				Logs:       out,
			})
			return
		}
		addResult(CheckResult{
			Name:       "yamllint",
			Status:     "passed",
			DurationMs: dur,
			Logs:       out,
		})
	}()

	// 5. MMv1 Core Compiler Unit Tests
	wg.Add(1)
	go func() {
		defer wg.Done()
		start := time.Now()
		out, err := runCommand(ctx, filepath.Join(repoRoot, "mmv1"), "go", "test", "./...")
		dur := time.Since(start).Milliseconds()
		if err != nil {
			addResult(CheckResult{
				Name:       "mmv1_unit",
				Status:     "failed",
				DurationMs: dur,
				Error:      "mmv1 unit tests failed",
				Logs:       out,
			})
			return
		}
		addResult(CheckResult{
			Name:       "mmv1_unit",
			Status:     "passed",
			DurationMs: dur,
			Logs:       out,
		})
	}()

	// 6. Internal Tools and CI Unit Tests (diff-scoped unless -all)
	type toolSuite struct {
		name       string
		pathPrefix string
		dir        string
		cmd        string
		args       []string
	}
	suites := []toolSuite{
		{name: "tools/go-changelog", pathPrefix: "tools/go-changelog/", dir: filepath.Join(repoRoot, "tools", "go-changelog"), cmd: "go", args: []string{"test", "./..."}},
		{name: "tools/issue-labeler", pathPrefix: "tools/issue-labeler/", dir: filepath.Join(repoRoot, "tools", "issue-labeler"), cmd: "go", args: []string{"test", "./..."}},
		{name: "tools/template-check", pathPrefix: "tools/template-check/", dir: filepath.Join(repoRoot, "tools", "template-check"), cmd: "go", args: []string{"test", "./..."}},
		{name: "tools/test-reader", pathPrefix: "tools/test-reader/", dir: filepath.Join(repoRoot, "tools", "test-reader"), cmd: "go", args: []string{"test", "./..."}},
		{name: ".ci/magician", pathPrefix: ".ci/", dir: filepath.Join(repoRoot, ".ci", "magician"), cmd: "go", args: []string{"test", "./..."}},
		{name: ".agents/scripts", pathPrefix: ".agents/scripts/", dir: repoRoot, cmd: "python3", args: []string{"-m", "unittest", "discover", "-s", ".agents/scripts", "-p", "test_*.py"}},
	}

	for _, s := range suites {
		s := s
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			if !*allFlag && !hasPathPrefix(changedFiles, s.pathPrefix) {
				addResult(CheckResult{
					Name:       s.name,
					Status:     "skipped",
					DurationMs: time.Since(start).Milliseconds(),
					Skipped:    true,
					SkipReason: fmt.Sprintf("no modified files in %s (use -all to run)", s.pathPrefix),
				})
				return
			}
			out, err := runCommand(ctx, s.dir, s.cmd, s.args...)
			dur := time.Since(start).Milliseconds()
			if err != nil {
				addResult(CheckResult{
					Name:       s.name,
					Status:     "failed",
					DurationMs: dur,
					Error:      fmt.Sprintf("%s unit tests failed", s.name),
					Logs:       out,
				})
				return
			}
			addResult(CheckResult{
				Name:       s.name,
				Status:     "passed",
				DurationMs: dur,
				Logs:       out,
			})
		}()
	}

	wg.Wait()
	totalDur := time.Since(t0)

	allPassed := true
	for _, r := range results {
		if r.Status == "failed" {
			allPassed = false
			break
		}
	}

	report := SummaryReport{
		Passed:     allPassed,
		BaseRef:    baseRef,
		RepoRoot:   repoRoot,
		DurationMs: totalDur.Milliseconds(),
		Checks:     results,
	}

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		if !allPassed {
			os.Exit(1)
		}
		return
	}

	fmt.Println()
	for _, r := range results {
		switch r.Status {
		case "passed":
			fmt.Printf("  \033[0;32m✓\033[0m %-25s (%4d ms)\n", r.Name, r.DurationMs)
			if *verboseFlag && strings.TrimSpace(r.Logs) != "" {
				fmt.Printf("    %s\n", strings.ReplaceAll(strings.TrimSpace(r.Logs), "\n", "\n    "))
			}
		case "skipped":
			fmt.Printf("  \033[0;33m-\033[0m %-25s (skipped: %s)\n", r.Name, r.SkipReason)
		case "failed":
			fmt.Printf("  \033[0;31m✗\033[0m %-25s (%4d ms) -> %s\n", r.Name, r.DurationMs, r.Error)
			if strings.TrimSpace(r.Logs) != "" {
				fmt.Printf("\n--- Failure Logs: %s ---\n%s\n------------------------\n\n", r.Name, strings.TrimSpace(r.Logs))
			}
		}
	}

	fmt.Println()
	if allPassed {
		fmt.Printf("\033[0;32m=== All pre-generation & static checks passed successfully! (%v) ===\033[0m\n", totalDur.Round(time.Millisecond))
		os.Exit(0)
	} else {
		fmt.Printf("\033[0;31m=== Pre-generation & static checks failed! (%v) ===\033[0m\n", totalDur.Round(time.Millisecond))
		os.Exit(1)
	}
}
