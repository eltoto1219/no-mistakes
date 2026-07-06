package cli

import (
	"fmt"
	"strings"

	toon "github.com/toon-format/toon-go"

	"github.com/kunchenguid/no-mistakes/internal/ipc"
	"github.com/kunchenguid/no-mistakes/internal/telemetry"
	"github.com/spf13/cobra"
)

func newAxiIntentCmd() *cobra.Command {
	var set string

	cmd := &cobra.Command{
		Use:   "intent",
		Short: "Show or edit the run's intent",
		Long: "Shows the intent attached to the run for the current branch (supplied at\n" +
			"`axi run --intent` or inferred from transcripts by the intent step). Pass\n" +
			"--set to replace it on the active run when it is wrong or stale: steps\n" +
			"that have not executed yet, and fix rounds of the step currently parked\n" +
			"at a gate, embed the edited intent in their prompts.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return trackAxiSurface("axi-intent", "/axi/intent", telemetry.Fields{
				"has_set": strings.TrimSpace(set) != "",
			}, func() error {
				return runAxiIntent(cmd, set)
			})
		},
	}
	cmd.Flags().StringVar(&set, "set", "", "replace the active run's intent with this text")
	return cmd
}

func runAxiIntent(cmd *cobra.Command, set string) error {
	ctx := cmd.Context()
	editing := strings.TrimSpace(set) != ""

	env, err := openAxiEnv(editing)
	if err != nil {
		return emitError(cmd, 1, err.Error(), repoInitHelp(err)...)
	}
	defer env.close()

	branch := currentBranchForRunResolve(ctx)

	if editing {
		return runAxiIntentSet(cmd, env, branch, strings.TrimSpace(set))
	}

	run, err := resolveRun(env, "", branch)
	if err != nil {
		return emitError(cmd, 1, err.Error())
	}
	if run == nil {
		return emitError(cmd, 1, "no runs yet in this repository",
			`Run `+"`"+`no-mistakes axi run --intent "<what the user set out to accomplish>"`+"`"+` to start one`)
	}
	emitDoc(cmd, intentFields(run.ID, run.Branch, derefOrEmpty(run.Intent), derefOrEmpty(run.IntentSource))...)
	return nil
}

func runAxiIntentSet(cmd *cobra.Command, env *axiEnv, branch, intent string) error {
	if branch == "" {
		return emitError(cmd, 1, "detached HEAD: check out a branch to edit its run's intent")
	}

	var active ipc.GetActiveRunResult
	if err := env.client.Call(ipc.MethodGetActiveRun, activeRunLookupParams(env.repo.ID, branch), &active); err != nil {
		return emitError(cmd, 1, fmt.Sprintf("get active run: %v", err))
	}
	if active.Run == nil {
		return emitError(cmd, 1, "no active run to edit",
			`Intent can only be edited on an active run; pass --intent to `+"`no-mistakes axi run`"+` when starting a new one`)
	}

	var result ipc.SetIntentResult
	if err := env.client.Call(ipc.MethodSetIntent, &ipc.SetIntentParams{RunID: active.Run.ID, Intent: intent}, &result); err != nil {
		return emitError(cmd, 1, fmt.Sprintf("set intent: %v", err))
	}
	if !result.OK {
		return emitError(cmd, 1, "daemon rejected the intent update")
	}

	fields := intentFields(active.Run.ID, active.Run.Branch, intent, "agent")
	fields = append(fields, toon.Field{Key: "help", Value: []string{
		"Steps that have not run yet (and fix rounds of the currently parked step) will use this intent",
	}})
	emitDoc(cmd, fields...)
	return nil
}

func intentFields(runID, branch, intent, source string) []toon.Field {
	fields := []toon.Field{
		{Key: "run", Value: runID},
		{Key: "branch", Value: branch},
	}
	if intent == "" {
		fields = append(fields, toon.Field{Key: "intent", Value: "none attached to this run"})
	} else {
		fields = append(fields, toon.Field{Key: "intent", Value: intent})
		if source != "" {
			fields = append(fields, toon.Field{Key: "source", Value: source})
		}
	}
	return fields
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
