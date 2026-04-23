package main

import (
	"errors"
	"fmt"
	job "github.com/bensyverson/jobs/internal/job"
	"os"
)

func main() {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, job.ErrTailTimeout) {
			fmt.Fprintln(os.Stderr, "Timeout:", err.Error())
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
		os.Exit(1)
	}
}
