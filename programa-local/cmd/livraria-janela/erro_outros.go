//go:build !windows

package main

import (
	"fmt"
	"os"
)

func mostrarErro(msg string) { fmt.Fprintln(os.Stderr, "erro:", msg) }
