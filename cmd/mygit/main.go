package main

import (
	"fmt"
	"os"
)

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

// Usage: your_program.sh <command> <arg1> <arg2> ...
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}

	switch command := os.Args[1]; command {
	case "init":
		must(initRepo())
	case "cat-file":
		must(catFile(args))
	case "hash-object":
		must(hashObject(args))
	case "ls-tree":
		must(lsTree(args))
	case "write-tree":
		must(writeTree())
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
