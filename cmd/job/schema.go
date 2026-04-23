package main

import (
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
)

func newSchemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the JSON Schema for `job import`",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return job.RunSchema(cmd.OutOrStdout())
		},
	}
	return cmd
}
