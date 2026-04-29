package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newEditCmd() *cobra.Command {
	var title string
	var desc string
	var criteria []string
	var setCriterion []string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change a task's title, description, or criteria",
		Long: "Replace a task's title and/or description, or update its acceptance criteria. " +
			"At least one of --title, --desc, --criterion, or --set-criterion must be provided. " +
			"Use --desc \"\" to clear the description.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDBFromCmd()
			if err != nil {
				return err
			}
			defer db.Close()

			actor, err := requireAs(db)
			if err != nil {
				return err
			}

			var titlePtr, descPtr *string
			if cmd.Flags().Changed("title") {
				t := title
				titlePtr = &t
			}
			if cmd.Flags().Changed("desc") {
				d := desc
				descPtr = &d
			}
			if titlePtr == nil && descPtr == nil && len(criteria) == 0 && len(setCriterion) == 0 {
				return fmt.Errorf("edit requires --title, --desc, --criterion, or --set-criterion")
			}

			if titlePtr != nil || descPtr != nil {
				if err := job.RunEdit(db, args[0], titlePtr, descPtr, actor); err != nil {
					return err
				}
			}
			if len(criteria) > 0 {
				items := make([]job.Criterion, 0, len(criteria))
				for _, label := range criteria {
					items = append(items, job.Criterion{Label: label})
				}
				if _, err := job.RunAddCriteria(db, args[0], items, actor); err != nil {
					return err
				}
			}
			for _, kv := range setCriterion {
				label, state, err := parseCriterionAssignment(kv)
				if err != nil {
					return err
				}
				if _, err := job.RunSetCriterion(db, args[0], label, state, actor); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Edited: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&title, "title", "t", "", "new title (replaces current)")
	cmd.Flags().StringVarP(&desc, "desc", "d", "", "new description (replaces current; pass \"\" to clear)")
	cmd.Flags().StringArrayVar(&criteria, "criterion", nil, "acceptance criterion to add, defaults to pending state (repeatable)")
	cmd.Flags().StringArrayVar(&setCriterion, "set-criterion", nil, "update an existing criterion's state, format \"label=passed\" (repeatable)")
	return cmd
}

// parseCriterionAssignment parses "label=state" into its parts and validates state.
func parseCriterionAssignment(raw string) (string, job.CriterionState, error) {
	idx := -1
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == '=' {
			idx = i
			break
		}
	}
	if idx <= 0 || idx == len(raw)-1 {
		return "", "", fmt.Errorf("invalid criterion assignment %q (want \"label=state\")", raw)
	}
	label := raw[:idx]
	stateRaw := raw[idx+1:]
	state, err := job.ValidateCriterionState(stateRaw)
	if err != nil {
		return "", "", err
	}
	return label, state, nil
}
