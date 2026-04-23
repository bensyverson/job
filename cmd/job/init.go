package main

import (
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

func newInitCmd() *cobra.Command {
	var force bool
	var writeGitignore bool
	var defaultIdentity string
	var strict bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new job database",
		Long: "Initialize a new .jobs.db in the current directory. Errors if one already exists unless --force is used.\n\n" +
			"By default, init records a default writer identity from $USER so subsequent writes don't need --as. Pass --default-identity <name> to pick a specific name, or --strict to opt out of default-identity convenience entirely (all writes will require --as). Use --gitignore to append recommended entries to .gitignore.",
		Args: cobra.NoArgs,
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
			defer db.Close()

			if force {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s (overwrote existing database)\n", path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized %s\n", path)
			}

			// Identity setup: --strict overrides everything, otherwise
			// --default-identity wins over $USER fallback. Writes under
			// strict mode must carry --as explicitly.
			if strict {
				if err := job.SetStrict(db, true); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Strict mode: writes require --as <name> (no default identity).")
			} else {
				name := defaultIdentity
				source := "--default-identity"
				if name == "" {
					name = os.Getenv("USER")
					source = "$USER"
				}
				if name != "" {
					if err := job.SetDefaultIdentity(db, name); err != nil {
						return err
					}
					fmt.Fprintf(cmd.OutOrStdout(), "Default identity: %s (from %s)\n", name, source)
				}
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
	cmd.Flags().StringVar(&defaultIdentity, "default-identity", "", "default writer identity (defaults to $USER)")
	cmd.Flags().BoolVar(&strict, "strict", false, "require --as on every write; do not set a default identity")
	return cmd
}
