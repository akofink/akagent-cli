package main

import (
	"os"

	"github.com/akofink/akagent-cli/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdout))
}
