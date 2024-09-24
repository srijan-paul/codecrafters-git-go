package main

import (
	"bytes"
	"compress/zlib"
	"io"
)

func decompress(r io.Reader) ([]byte, error) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}

	return out, nil
}

func compress(w io.Writer, data []byte) error {
	zw := zlib.NewWriter(w)
	defer zw.Close()

	_, err := zw.Write(data)
	return err
}

func splitOn(b []byte, sep byte) ([]byte, []byte) {
	i := bytes.IndexByte(b, sep)
	if i == -1 {
		return b, nil
	}
	return b[:i], b[i+1:]
}

