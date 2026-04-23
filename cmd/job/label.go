package main

import "github.com/spf13/cobra"

func newLabelCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "label",
		Short: "Add or remove labels on a task",
		Long:  "Manage flat, free-form labels on a task. Subcommands: 'add' and 'remove'. Names are variadic per call, idempotent, and atomic. Labels are local to each task (no inheritance). Reserved: \"decision\" — surfaces as a Decision: line in `job status` until done or canceled.",
	}
	cmd.AddCommand(newLabelAddCmd())
	cmd.AddCommand(newLabelRemoveCmd())
	return cmd
}
