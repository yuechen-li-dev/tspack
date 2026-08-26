package main

import (
	"os"

	"github.com/yuechen-li-dev/tspack/internal/cli"
)

func main() {
	cli.Main(os.Args[1:])
}
