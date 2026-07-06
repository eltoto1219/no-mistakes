package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/types"
)

func writeGlobalConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadGlobal_CommitFixMessageAndPRSignatureDefaults(t *testing.T) {
	cfg, err := LoadGlobal("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CommitFixMessage != "" {
		t.Errorf("commit.fix_message = %q, want empty", cfg.CommitFixMessage)
	}
	if cfg.HidePRSignature {
		t.Error("HidePRSignature = true, want false by default (signature shown)")
	}
}

func TestLoadGlobal_CommitFixMessageFromFile(t *testing.T) {
	path := writeGlobalConfig(t, "commit:\n  fix_message: \"chore({{.Step}}): {{.Summary}}\"\n")
	cfg, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := cfg.CommitFixMessage, "chore({{.Step}}): {{.Summary}}"; got != want {
		t.Errorf("commit.fix_message = %q, want %q", got, want)
	}
}

func TestLoadGlobal_CommitFixMessageInvalidTemplate(t *testing.T) {
	path := writeGlobalConfig(t, "commit:\n  fix_message: \"chore({{.Step): {{.Summary}}\"\n")
	if _, err := LoadGlobal(path); err == nil {
		t.Fatal("expected error for unparseable template, got nil")
	} else if !strings.Contains(err.Error(), "fix_message") {
		t.Errorf("error %q does not mention fix_message", err)
	}
}

func TestLoadGlobal_CommitFixMessageUnknownField(t *testing.T) {
	path := writeGlobalConfig(t, "commit:\n  fix_message: \"{{.Nope}}: {{.Summary}}\"\n")
	if _, err := LoadGlobal(path); err == nil {
		t.Fatal("expected error for unknown template field, got nil")
	}
}

func TestLoadGlobal_CommitFixMessageValidatesEveryFixStep(t *testing.T) {
	fixSteps := []types.StepName{
		types.StepReview,
		types.StepTest,
		types.StepLint,
		types.StepDocument,
		types.StepCI,
		types.StepPush,
	}
	for _, step := range fixSteps {
		t.Run(string(step), func(t *testing.T) {
			tmpl := fmt.Sprintf(`{{if eq .Step %q}}{{.Nope}}{{else}}ok{{end}}`, step)
			path := writeGlobalConfig(t, "commit:\n  fix_message: '"+tmpl+"'\n")
			if _, err := LoadGlobal(path); err == nil {
				t.Fatalf("expected error for invalid %s conditional branch, got nil", step)
			}
		})
	}

	t.Run("empty conditional branch", func(t *testing.T) {
		tmpl := `{{if eq .Step "push"}} {{else}}ok{{end}}`
		path := writeGlobalConfig(t, "commit:\n  fix_message: '"+tmpl+"'\n")
		if _, err := LoadGlobal(path); err == nil {
			t.Fatal("expected error for empty conditional branch, got nil")
		}
	})
}

func TestLoadGlobal_CommitFixMessageWhitespaceOnlyRender(t *testing.T) {
	path := writeGlobalConfig(t, "commit:\n  fix_message: \"  \"\n")
	if _, err := LoadGlobal(path); err == nil {
		t.Fatal("expected error for whitespace-only message, got nil")
	}
}

func TestLoadGlobal_PRSignatureFalse(t *testing.T) {
	path := writeGlobalConfig(t, "pr_signature: false\n")
	cfg, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.HidePRSignature {
		t.Error("HidePRSignature = false, want true for pr_signature: false")
	}
}

func TestMerge_CommitFixMessageAndPRSignature(t *testing.T) {
	global := &GlobalConfig{
		CommitFixMessage: "fix({{.Step}}): {{.Summary}}",
		HidePRSignature:  true,
	}
	cfg := Merge(global, &RepoConfig{})
	if got, want := cfg.CommitFixMessage, "fix({{.Step}}): {{.Summary}}"; got != want {
		t.Errorf("CommitFixMessage = %q, want %q", got, want)
	}
	if !cfg.HidePRSignature {
		t.Error("HidePRSignature = false, want true")
	}
}

func TestConfig_FixCommitMessage(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		step    types.StepName
		summary string
		want    string
		wantOK  bool
	}{
		{
			name:   "unset template",
			cfg:    &Config{},
			step:   types.StepLint,
			wantOK: false,
		},
		{
			name:   "nil config",
			cfg:    nil,
			step:   types.StepLint,
			wantOK: false,
		},
		{
			name:    "renders step and summary",
			cfg:     &Config{CommitFixMessage: "chore({{.Step}}): {{.Summary}}"},
			step:    types.StepReview,
			summary: "address review findings",
			want:    "chore(review): address review findings",
			wantOK:  true,
		},
		{
			name:    "empty summary falls back to apply fixes",
			cfg:     &Config{CommitFixMessage: "fix: {{.Summary}} [{{.Step}}]"},
			step:    types.StepCI,
			summary: "",
			want:    "fix: apply fixes [ci]",
			wantOK:  true,
		},
		{
			name:    "render error returns not ok",
			cfg:     &Config{CommitFixMessage: "{{.Nope}}"},
			step:    types.StepLint,
			summary: "x",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.cfg.FixCommitMessage(tt.step, tt.summary)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}
