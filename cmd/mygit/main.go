package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var ObjectsDir = ".git/objects"

type ObjectMode string

var (
	ObjectModeFile    ObjectMode = "100644"
	ObjectModeExe                = "100755"
	ObjectModeSymlink            = "120000"
	ObjectModeDir     ObjectMode = "40000"
)

type TreeObjectEntry struct {
	shaHash  []byte
	FileName string
	Mode     ObjectMode
}

func stringifyEntry(objectEntry *TreeObjectEntry, nameOnly bool) string {
	if nameOnly {
		return fmt.Sprintf("%s", objectEntry.FileName)
	}

	return fmt.Sprintf(
		"%s %s %x",
		objectEntry.Mode,
		objectEntry.FileName,
		objectEntry.shaHash,
	)
}

type TreeObject struct {
	ShaHash string
	DirName string
	Entries []*TreeObjectEntry
}

func SplitOn(b []byte, sep byte) ([]byte, []byte) {
	i := bytes.IndexByte(b, sep)
	if i == -1 {
		return b, nil
	}
	return b[:i], b[i+1:]
}

func parseNextEntry(treeBody []byte) (*TreeObjectEntry, []byte) {
	objectHdr, rest := SplitOn(treeBody, 0)
	modeBytes, nameBytes := SplitOn(objectHdr, ' ')

	if len(modeBytes) == 0 || len(nameBytes) == 0 {
		panic("Invalid tree object") // todo: improve error, and bubble up
	}

	mode := ObjectMode(modeBytes)
	name := string(nameBytes)

	hashBytes := rest[:20]
	nextBytes := rest[20:]

	object := &TreeObjectEntry{
		shaHash:  hashBytes,
		Mode:     mode,
		FileName: name,
	}

	return object, nextBytes
}

func parseTreeObject(content []byte) ([]*TreeObjectEntry, error) {
	var entries []*TreeObjectEntry

	header, body := SplitOn(content, 0)
	if len(header) == 0 || len(body) == 0 {
		return nil, fmt.Errorf(
			"Invalid tree object (hdr len: %d, body len: %d)",
			len(header),
			len(body),
		)
	}

	headerStr := string(header)

	// we only use the header for validation right now
	if !strings.HasPrefix(headerStr, "tree ") {
		return nil, fmt.Errorf("Invalid tree object")
	}

	// parse body
	for len(body) > 0 {
		parsedEntry, rest := parseNextEntry(body)
		entries = append(entries, parsedEntry)
		body = rest
	}

	return entries, nil
}

func filePathFromObjectHash(hash string) string {
	return filepath.Join(ObjectsDir, hash[:2], hash[2:])
}

func decompress(r io.Reader) []byte {
	zr, err := zlib.NewReader(r)
	must(err)
	defer zr.Close()

	out, err := io.ReadAll(zr)
	must(err)

	return out
}

func serializeTreeObject(entries []*TreeObjectEntry) []byte {
	var buf bytes.Buffer
	for _, entry := range entries {
		buf.WriteString(string(entry.Mode))
		buf.WriteByte(' ')
		buf.WriteString(entry.FileName)
		buf.WriteByte(0)
		buf.Write(entry.shaHash)
	}

	bytes := buf.Bytes()

	// prepend header

	header := fmt.Sprintf("tree %d\x00", len(bytes))
	return append([]byte(header), bytes...)
}

func treeFromDir(dir string) (*TreeObjectEntry, error) {
	var entries []*TreeObjectEntry
	dir = filepath.Clean(dir)

	processFile := func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == dir {
			return nil
		}

		fileOrDirName := filepath.Base(path)
		if fileOrDirName == ".git" {
			return filepath.SkipDir
		}

		path, err = filepath.Rel(".", path)
		if err != nil {
			return err
		}

		if f.IsDir() {
			entry, err := treeFromDir(path)
			if err != nil {
				return err
			}

			if entry != nil {
				entries = append(entries, entry)
			}

			return filepath.SkipDir 
		}

		object, err := objectFromFile(path)
		if err != nil {
			return err
		}

		entries = append(entries, object)
		return nil
	}

	err := filepath.Walk(dir, processFile)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, nil
	}

	slices.SortFunc(entries, func(o1, o2 *TreeObjectEntry) int {
		return strings.Compare(o1.FileName, o2.FileName)
	})

	contents := serializeTreeObject(entries)
	hash := sha1.Sum(contents)

	tree := &TreeObjectEntry{
		shaHash:  hash[:],
		FileName: dir,
		Mode:     ObjectModeDir,
	}

	// write to disk
	treeFilePath := filePathFromObjectHash(fmt.Sprintf("%x", hash))
	dirName := filepath.Dir(treeFilePath)
	if err := os.MkdirAll(dirName, os.ModePerm); err != nil {
		return nil, err
	}

	treeFile, err := os.Create(treeFilePath)
	if err != nil {
		return nil, err
	}

	defer treeFile.Close()

	zw := zlib.NewWriter(treeFile)
	defer zw.Close()

	_, err = zw.Write(contents)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

func writeTree(_ []string) error {
	t, err := treeFromDir(".")
	if err != nil {
		return err
	}

	if t != nil {
		fmt.Println(fmt.Sprintf("%x", t.shaHash))
	}

	return nil
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

	treeContents := decompress(treeFile)

	tree, err := parseTreeObject(treeContents)
	if err != nil {
		return err
	}

	cmpObjects := func(o1, o2 *TreeObjectEntry) int {
		return strings.Compare(o1.FileName, o2.FileName)
	}

	slices.SortFunc(tree, cmpObjects)
	for _, entry := range tree {
		fmt.Println(stringifyEntry(entry, nameOnly))
	}

	return nil
}

func objectFromFile(filePath string) (*TreeObjectEntry, error) {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	blobSize := fmt.Sprintf("blob %d", len(contents))
	objectBytes := bytes.Join([][]byte{[]byte(blobSize), contents}, []byte{0})
	hash := sha1.Sum(objectBytes)

	shaSumStr := fmt.Sprintf("%x", hash[:])
	dstPath := filePathFromObjectHash(shaSumStr)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return nil, err
	}

	dstF, err := os.Create(dstPath)
	if err != nil {
		return nil, err
	}
	defer dstF.Close()

	zwriter := zlib.NewWriter(dstF)
	defer zwriter.Close()

	_, err = zwriter.Write(objectBytes)
	if err != nil {
		return nil, err
	}

	return &TreeObjectEntry{
		shaHash:  hash[:],
		Mode:     ObjectModeFile,
		FileName: filepath.Base(filePath),
	}, nil
}

func hashObject(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("Expected 2 arguments")
	}

	if args[0] != "-w" {
		return fmt.Errorf("Expected -w flag")
	}

	filePath := args[1]
	object, err := objectFromFile(filePath)
	if err != nil {
		return err
	}

	shaSumStr := fmt.Sprintf("%x", object.shaHash)
	fmt.Print(shaSumStr)
	return nil
}

// TODO: use a parse helper function here instead.
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

	decompressedBytes := decompress(file)
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
		for _, dir := range []string{ObjectsDir, ".git/refs"} {
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

	case "ls-tree":
		err := lsTree(args)
		must(err)
	case "write-tree":
		err := writeTree(args)
		must(err)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}
}
