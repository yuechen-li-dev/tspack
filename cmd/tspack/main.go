package main

import (
	"context"
	"os"

	"github.com/yuechen-li-dev/tspack/internal/cli"
)

func main() {
	app := cli.NewDefaultApp()
	code := app.Run(context.Background(), os.Args[1:])
	os.Exit(code)
}
