package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ncecere/agent/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
