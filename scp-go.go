package main

import (
	"fmt"
	"github.com/laher/scp-go/scp"
	"os"
)

func main() {
	scper := new(scp.SecureCopier)
	err := scper.ParseFlags(os.Args, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	err = scper.Exec(os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

}
