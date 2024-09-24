package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func hashObject(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("Expected 2 arguments")
	}

	if args[0] != "-w" {
		return fmt.Errorf("Expected -w flag")
	}

	filePath := args[1]


	contents, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	blobSize := fmt.Sprintf("blob %d", len(contents))
	objectBytes:= bytes.Join([][]byte { []byte(blobSize), contents}, []byte{0})
	hash := sha1.Sum(objectBytes)

	shaSumStr := fmt.Sprintf("%x", hash[:])
	dstPath := filepath.Join(".git/objects", shaSumStr[:2], shaSumStr[2:])
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err	
	}
	
	dstF, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dstF.Close()
	
	zwriter := zlib.NewWriter(dstF)
	defer zwriter.Close()

	_, err = zwriter.Write(objectBytes)
	if err != nil {
		return err
	}

	fmt.Print(shaSumStr)
	return nil
}


func catFile(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: mygit cat-file -p <hash>")
	}

	if args[0] != "-p" {
		return fmt.Errorf("Unknown option -o")
	}

	hash := args[1]
	basePath := ".git/objects"

	if len(hash) < 2 {
		return fmt.Errorf("Hash too short")
	}

	dir := hash[:2]
	fileName := hash[2:]
	filePath := filepath.Join(basePath, dir, fileName)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	z, err := zlib.NewReader(file)
	if err != nil {
		return err
	}

	decompressedBytes, err := io.ReadAll(z)
	if err != nil {
		return err
	}

	delimIndex := bytes.Index(decompressedBytes, []byte{0})
	if delimIndex == -1 {
		return fmt.Errorf("Invalid object")
	}

	payloadBytes := decompressedBytes[delimIndex+1:] 
	payload := string(payloadBytes)

	fmt.Print(payload)
	return nil
}

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
		for _, dir := range []string{".git/objects", ".git/refs"} {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating directory: %s\n", err)
				os.Exit(1)
			}
		}

		headFileContents := []byte("ref: refs/heads/main\n")
		if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
			os.Exit(1)
		}

		fmt.Println("Initialized git directory")

	case "cat-file":
		err := catFile(args)
		must(err)

	case "hash-object":
		err := hashObject(args)
		must(err)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
