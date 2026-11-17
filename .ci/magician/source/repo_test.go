package source

import (
	"reflect"
	"testing"
)

type recordingRunner struct {
	calls [][]string
}

func (r *recordingRunner) PushDir(path string) error { return nil }
func (r *recordingRunner) PopDir() error             { return nil }
func (r *recordingRunner) Run(name string, args []string, env map[string]string) (string, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	return "", nil
}

func TestControllerGitArgsAndRefValidation(t *testing.T) {
	t.Run("valid refs use end-of-options separator", func(t *testing.T) {
		rnr := &recordingRunner{}
		ctlr := NewController("/go", "modular-magician", "tok", rnr)

		repoNoBranch := &Repo{Name: "tpg", Owner: "hashicorp", Path: "/go/tpg"}
		if err := ctlr.Clone(repoNoBranch); err != nil {
			t.Fatalf("Clone(empty branch) unexpected error: %v", err)
		}

		repo := &Repo{Name: "tpgb", Owner: "hashicorp", Branch: "main", Path: "/go/tpgb"}
		if err := ctlr.Clone(repo); err != nil {
			t.Fatalf("Clone(main) unexpected error: %v", err)
		}
		if err := ctlr.Checkout(repo, "HEAD~"); err != nil {
			t.Fatalf("Checkout(HEAD~) unexpected error: %v", err)
		}
		if err := ctlr.Fetch(repo, "auto-pr-123-old"); err != nil {
			t.Fatalf("Fetch() unexpected error: %v", err)
		}
		if _, err := ctlr.DiffShortStat(repo, "auto-pr-123-old", "auto-pr-123"); err != nil {
			t.Fatalf("DiffShortStat() unexpected error: %v", err)
		}
		if _, err := ctlr.DiffNameOnly(repo, "auto-pr-123-old", "auto-pr-123"); err != nil {
			t.Fatalf("DiffNameOnly() unexpected error: %v", err)
		}
		if _, err := ctlr.DiffUnifiedZero(repo, "auto-pr-123-old", "auto-pr-123"); err != nil {
			t.Fatalf("DiffUnifiedZero() unexpected error: %v", err)
		}

		want := [][]string{
			{"git", "clone", "--", "https://modular-magician:tok@github.com/hashicorp/tpg", "/go/tpg"},
			{"git", "clone", "-b", "main", "--", "https://modular-magician:tok@github.com/hashicorp/tpgb", "/go/tpgb"},
			{"git", "checkout", "HEAD~", "--"},
			{"git", "fetch", "--", "origin", "auto-pr-123-old"},
			{"git", "diff", "--shortstat", "origin/auto-pr-123-old", "origin/auto-pr-123", "--"},
			{"git", "diff", "--name-only", "origin/auto-pr-123-old", "origin/auto-pr-123", "--"},
			{"git", "diff", "--unified=0", "origin/auto-pr-123-old", "origin/auto-pr-123", "--"},
		}
		if !reflect.DeepEqual(rnr.calls, want) {
			t.Errorf("got calls %v, want %v", rnr.calls, want)
		}
	})

	t.Run("rejects leading-dash and whitespace refs without invoking git", func(t *testing.T) {
		rnr := &recordingRunner{}
		ctlr := NewController("/go", "modular-magician", "tok", rnr)
		repo := &Repo{Name: "tpgb", Owner: "hashicorp", Branch: "--upload-pack=evil", Path: "/go/tpgb"}

		if err := ctlr.Clone(repo); err == nil {
			t.Error("Clone(--upload-pack=evil) succeeded, want error")
		}
		if err := ctlr.Checkout(repo, "-f"); err == nil {
			t.Error("Checkout(-f) succeeded, want error")
		}
		if err := ctlr.Fetch(repo, "--upload-pack=evil"); err == nil {
			t.Error("Fetch(--upload-pack=evil) succeeded, want error")
		}
		if _, err := ctlr.DiffShortStat(repo, "-o", "main"); err == nil {
			t.Error("DiffShortStat(-o, main) succeeded, want error")
		}
		if _, err := ctlr.DiffNameOnly(repo, "main", "bad ref"); err == nil {
			t.Error("DiffNameOnly(main, 'bad ref') succeeded, want error")
		}
		if _, err := ctlr.DiffUnifiedZero(repo, "", "main"); err == nil {
			t.Error("DiffUnifiedZero('', main) succeeded, want error")
		}
		if len(rnr.calls) != 0 {
			t.Errorf("expected 0 git calls on invalid refs, got %v", rnr.calls)
		}
	})
}
