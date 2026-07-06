package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/db"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

func TestCommitAgentFixes_UsesConfiguredTemplate(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)

	ag := &mockAgent{name: "test"}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.Config.CommitFixMessage = "chore({{.Step}}): {{.Summary}}"

	os.WriteFile(filepath.Join(dir, "agent-change.txt"), []byte("change"), 0o644)
	if err := commitAgentFixes(sctx, types.StepLint, "fix trailing whitespace", "fallback"); err != nil {
		t.Fatal(err)
	}
	if got := lastCommitMessage(t, dir); got != "chore(lint): fix trailing whitespace" {
		t.Errorf("commit message = %q, want templated message", got)
	}
}

func TestPushStep_UsesConfiguredTemplateForFixCommit(t *testing.T) {
	t.Parallel()
	upstream := t.TempDir()
	gitCmd(t, upstream, "init", "--bare")

	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.name", "test")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	gitCmd(t, dir, "remote", "add", "origin", upstream)
	gitCmd(t, dir, "push", "origin", "main")

	gitCmd(t, dir, "checkout", "-b", "feature")
	os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature"), 0o644)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "feature")
	headSHA := gitCmd(t, dir, "rev-parse", "HEAD")
	baseSHA := gitCmd(t, dir, "rev-parse", "main")
	gitCmd(t, dir, "push", "origin", "feature")

	// Uncommitted agent changes force the push step's fix commit.
	os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("agent fix"), 0o644)

	ag := &mockAgent{name: "test"}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.Repo.UpstreamURL = upstream
	sctx.Config.CommitFixMessage = "chore({{.Step}}): {{.Summary}}"

	step := &PushStep{}
	if _, err := step.Execute(sctx); err != nil {
		t.Fatal(err)
	}
	if got := lastCommitMessage(t, dir); got != "chore(push): apply agent fixes" {
		t.Errorf("commit message = %q, want templated push message", got)
	}
}

func TestCIStep_CommitAndPush_UsesConfiguredTemplate(t *testing.T) {
	t.Parallel()
	upstream := t.TempDir()
	gitCmd(t, upstream, "init", "--bare")

	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.name", "test")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "checkout", "-b", "main")
	os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	baseSHA := gitCmd(t, dir, "rev-parse", "HEAD")
	gitCmd(t, dir, "remote", "add", "origin", upstream)
	gitCmd(t, dir, "push", "origin", "main")

	gitCmd(t, dir, "checkout", "-b", "feature")
	os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("feature"), 0o644)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "feature")
	headSHA := gitCmd(t, dir, "rev-parse", "HEAD")
	gitCmd(t, dir, "push", "origin", "feature")

	os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("ci fix"), 0o644)

	ag := &mockAgent{name: "test"}
	sctx := newTestContext(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.Repo.UpstreamURL = upstream
	sctx.Run.Branch = "refs/heads/feature"
	sctx.Config.CommitFixMessage = "chore({{.Step}}): {{.Summary}}"

	step := &CIStep{}
	pushed, err := step.commitAndPush(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if !pushed {
		t.Error("expected commitAndPush to report changes were pushed")
	}
	if got := lastCommitMessage(t, dir); got != "chore(ci): apply CI fixes" {
		t.Errorf("commit message = %q, want templated CI message", got)
	}
}

func TestBuildPipelineSummary_SignatureToggle(t *testing.T) {
	t.Parallel()
	steps := []*db.StepResult{
		{ID: "s1", StepName: types.StepReview, Status: types.StepStatusCompleted},
	}
	rounds := map[string][]*db.StepRound{
		"s1": {{Round: 1, Trigger: "initial", DurationMS: 500}},
	}

	md, _ := BuildPipelineSummary(steps, rounds, false)
	if strings.Contains(md, "no-mistakes") {
		t.Errorf("expected no no-mistakes reference with signature disabled, got:\n%s", md)
	}
	if !strings.Contains(md, "## Pipeline") {
		t.Errorf("missing Pipeline heading, got:\n%s", md)
	}

	md, _ = BuildPipelineSummary(steps, rounds, true)
	if !strings.Contains(md, noMistakesPRSignature) {
		t.Errorf("expected signature with pr_signature enabled, got:\n%s", md)
	}
}
