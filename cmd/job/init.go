package main

import (
	"fmt"
	job "github.com/bensyverson/job/internal/job"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

func newInitCmd() *cobra.Command {
	var force bool
	var writeGitignore bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new job database",
		Long:  "Initialize a new .jobs.db in the current directory. Errors if one already exists unless --force is used. Use --gitignore to append recommended entries to .gitignore.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := job.ResolveDBPathForInit(dbPath)
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists. Use --force to overwrite", path)
			}
			if force {
				os.Remove(path)
			}
			db, err := job.CreateDB(path)
			if err != nil {
				return err
			}
			db.Close()
			if force {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s (overwrote existing database)\n", path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s\n", path)
			}

			if writeGitignore {
				dir := filepath.Dir(path)
				if dir == "" {
					dir = "."
				}
				written, alreadyPresent, gerr := job.WriteGitignoreEntries(dir)
				if gerr != nil {
					return gerr
				}
				if len(written) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d entries to .gitignore: %s\n", len(written), strings.Join(written, ", "))
				} else if len(alreadyPresent) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), ".gitignore already includes %s\n", humanJoin(alreadyPresent))
				}
			} else {
				fmt.Fprintln(cmd.OutOrStdout())
				fmt.Fprintln(cmd.OutOrStdout(), job.GitignoreHint)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing database")
	cmd.Flags().BoolVar(&writeGitignore, "gitignore", false, "append recommended entries to .gitignore")
	return cmd
}
