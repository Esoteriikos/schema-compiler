package main

import (
	"os"

	"github.com/Esoteriikos/schema-compiler/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
