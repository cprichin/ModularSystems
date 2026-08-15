package main

import (
	"fmt"
	"modularsystems/config"
	"os"
)

var ENVPATH string = "..\\.env"
var cfg config.Config

func main() {

	cfg, warn, err := config.Load(ENVPATH)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration is invalid:")
		fmt.Fprintln(os.Stderr, err)

	}
	for _, w := range warn {
		fmt.Fprintln(os.Stderr, w)
	}
	if err != nil {
		os.Exit(1)
	}
	fmt.Print(cfg)
}
