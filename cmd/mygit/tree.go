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

var ObjectsDir = ".git" + string(os.PathSeparator) + "objects"

type ObjectMode string
const (
	ObjectModeFile    ObjectMode = "100644"
	ObjectModeExe                = "100755"
	ObjectModeSymlink            = "120000"
	ObjectModeDir                = "40000"
)

type ObjKind int

const (
	ObjKindBlob ObjKind = iota
	ObjKindTree
	ObjKindCommit // (commit-tree not implemented yet)
)

type Object struct {
	Kind     ObjKind
	// when including commits,
	// this will be a "tagged union":
	// with fields `.Commit`, `.Tree`, and `.Blob` fields.
	// the `.Tree` can have `.Entries`,
	// and `.Commit` can have `.Message` and `.Author`, etc.
	ShaHash  []byte
	FileName string
	Mode     ObjectMode
}

func (t *Object) writeToDisk(fileContents []byte) error {
	treeFilePath := filePathFromObjectHash(fmt.Sprintf("%x", t.ShaHash))
	dirName := filepath.Dir(treeFilePath)
	if err := os.MkdirAll(dirName, os.ModePerm); err != nil {
		return err
	}

	treeFile, err := os.Create(treeFilePath)
	if err != nil {
		return err
	}
	defer treeFile.Close()

	return compress(treeFile, fileContents)
}


// Parse the next entry in a tree object
func parseNextTreeEntry(treeBody []byte) (*Object, []byte, error) {
	objectHdr, rest := splitOn(treeBody, 0)
	modeBytes, nameBytes := splitOn(objectHdr, ' ')

	if len(modeBytes) == 0 || len(nameBytes) == 0 {
		return nil, nil, fmt.Errorf("mode or name missing")
	}

	mode := ObjectMode(modeBytes)
	name := string(nameBytes)

	hashBytes := rest[:20]
	nextBytes := rest[20:]

	objKind := ObjKindBlob
	if strings.HasPrefix(name, "tree") {
		objKind = ObjKindTree
	} else if strings.HasPrefix(name, "commit") {
		objKind = ObjKindCommit
	} else { // supporting tags will need us to add another branch here.
		objKind = ObjKindBlob
	}

	object := &Object{
		Kind:     objKind,
		ShaHash:  hashBytes,
		Mode:     mode,
		FileName: name,
	}

	return object, nextBytes, nil
}

func parseTreeObject(treeBytes []byte) ([]*Object, error) {
	var entries []*Object

	headerBytes, body := splitOn(treeBytes, 0)
	if len(headerBytes) == 0 || len(body) == 0 {
		return nil, fmt.Errorf(
			"Invalid tree object (hdr len: %d, body len: %d)",
			len(headerBytes),
			len(body),
		)
	}

	// We only use the header for validation when parsing
	header := string(headerBytes)
	if !strings.HasPrefix(header, "tree ") {
		return nil, fmt.Errorf("Invalid tree object")
	}

	// Parse the entries in the body until we run out of bytes
	for len(body) > 0 {
		parsedEntry, rest, err := parseNextTreeEntry(body)
		if err != nil {
			return nil, err
		}

		entries = append(entries, parsedEntry)
		body = rest
	}

	return entries, nil
}

// return the path to an object file, given its hash
func filePathFromObjectHash(hash string) string {
	return filepath.Join(ObjectsDir, hash[:2], hash[2:])
}

// Given the entries of a tree, serialize it to a byte slice.
func serializeTreeObject(entries []*Object) []byte {
	// 1. populate body
	var buf bytes.Buffer
	for _, entry := range entries {
		buf.WriteString(string(entry.Mode))
		buf.WriteByte(' ')
		buf.WriteString(entry.FileName)
		buf.WriteByte(0)
		buf.Write(entry.ShaHash)
	}
	body := buf.Bytes()

	// 2. prepend header
	header := fmt.Sprintf("tree %d\x00", len(body))
	return append([]byte(header), body...)
}

// From a directory path, create a tree object.
// This object is also written to a file on disk.
func createTreeFromDir(dir string) (*Object, error) {
	var entries []*Object
	dir = filepath.Clean(dir)

	processFile := func(path string, f os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// do not recurse into the current dir
		if path == dir {
			return nil
		}

		if filepath.Base(path) == ".git" {
			return filepath.SkipDir
		}

		path, err = filepath.Rel(".", path)
		if err != nil {
			return err
		}

		if f.IsDir() {
			entry, err := createTreeFromDir(path)
			if err != nil {
				return err
			}

			if entry != nil {
				entries = append(entries, entry)
			}

			return filepath.SkipDir
		}

		object, err := createObjectFromFile(path)
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

	slices.SortFunc(entries, func(o1, o2 *Object) int {
		return strings.Compare(o1.FileName, o2.FileName)
	})

	contents := serializeTreeObject(entries)
	hash := sha1.Sum(contents)

	tree := &Object{
		ShaHash:  hash[:],
		FileName: dir,
		Mode:     ObjectModeDir,
	}

	err = tree.writeToDisk(contents)
	if err != nil {
		return nil, err
	}

	return tree, nil
}


