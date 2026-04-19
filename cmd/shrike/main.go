package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("shrike dev")
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "shrike: not implemented yet")
	os.Exit(1)
}
