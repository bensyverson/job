package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		if errors.Is(err, errTailTimeout) {
			fmt.Fprintln(os.Stderr, "Timeout:", err.Error())
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
		os.Exit(1)
	}
}
