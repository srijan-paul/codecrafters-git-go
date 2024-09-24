package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func writeTree() error {
	t, err := createTreeFromDir(".")
	if err != nil {
		return err
	}

	if t != nil {
		fmt.Println(fmt.Sprintf("%x", t.ShaHash))
	}

	return nil
}

func stringifyObject(objectEntry *Object, nameOnly bool) string {
	if nameOnly {
		return objectEntry.FileName
	}

	return fmt.Sprintf(
		"%s %s %x",
		objectEntry.Mode,
		objectEntry.FileName,
		objectEntry.ShaHash,
	)
}


func lsTree(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mygit ls-tree [--name-only] <hash>")
	}

	var nameOnly bool
	if args[0] == "--name-only" {
		nameOnly = true
		args = args[1:]
	}

	if len(args) != 1 {
		return fmt.Errorf("usage: mygit ls-tree [--name-only] <hash>")
	}

	treeHash := args[0]

	treeFilePath := filePathFromObjectHash(treeHash)
	treeFile, err := os.Open(treeFilePath)
	if err != nil {
		return err
	}
	defer treeFile.Close()

	treeContents, err := decompress(treeFile)
	if err != nil {
		return err
	}

	tree, err := parseTreeObject(treeContents)
	if err != nil {
		return err
	}

	cmpObjects := func(o1, o2 *Object) int {
		return strings.Compare(o1.FileName, o2.FileName)
	}

	slices.SortFunc(tree, cmpObjects)
	for _, entry := range tree {
		fmt.Println(stringifyObject(entry, nameOnly))
	}

	return nil
}

func createObjectFromFile(filePath string) (*Object, error) {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	blobSize := fmt.Sprintf("blob %d", len(contents))
	objectBytes := bytes.Join([][]byte{[]byte(blobSize), contents}, []byte{0})
	hash := sha1.Sum(objectBytes)

	object := &Object{
		ShaHash:  hash[:],
		Mode:     ObjectModeFile,
		FileName: filepath.Base(filePath),
	}

	return object, object.writeToDisk(objectBytes)
}

func hashObject(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("Expected 2 arguments")
	}

	if args[0] != "-w" {
		return fmt.Errorf("Expected -w flag")
	}

	filePath := args[1]
	object, err := createObjectFromFile(filePath)
	if err != nil {
		return err
	}

	shaSumStr := fmt.Sprintf("%x", object.ShaHash)
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
	if len(hash) < 2 {
		return fmt.Errorf("Hash too short")
	}

	filePath := filePathFromObjectHash(hash)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	defer file.Close()

	decompressedBytes, err := decompress(file)
	if err != nil {
		return err
	}

	delimIndex := bytes.IndexByte(decompressedBytes, 0)
	if delimIndex == -1 {
		return fmt.Errorf("Invalid object")
	}

	payloadBytes := decompressedBytes[delimIndex+1:]
	payload := string(payloadBytes)

	fmt.Print(payload)
	return nil
}

func initRepo() error {
	for _, dir := range []string{ObjectsDir, ".git/refs"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	headFileContents := []byte("ref: refs/heads/main\n")
	if err := os.WriteFile(".git/HEAD", headFileContents, 0644); err != nil {
		return err
	}

	return nil
}
