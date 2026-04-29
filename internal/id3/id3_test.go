package id3

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTBPMCreatesID3Tag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "track.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteTBPM(path, 123); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:3]) != "ID3" {
		t.Fatalf("missing ID3 header")
	}
	if string(data[10:14]) != "TBPM" {
		t.Fatalf("missing TBPM frame")
	}
}

func TestWriteTBPMReplacesExistingFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "track.mp3")
	tag := append([]byte{
		'I', 'D', '3', 3, 0, 0, 0, 0, 0, 15,
		'T', 'B', 'P', 'M', 0, 0, 0, 5, 0, 0, 0, '9', '9', '9', '9',
	}, []byte("audio")...)
	if err := os.WriteFile(path, tag, 0644); err != nil {
		t.Fatal(err)
	}

	if err := WriteTBPM(path, 128); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[10:14]) != "TBPM" {
		t.Fatalf("missing TBPM frame")
	}
	if string(data[21:24]) != "128" {
		t.Fatalf("unexpected BPM payload: %q", data[21:24])
	}
	if count := countSubstr(data, []byte("TBPM")); count != 1 {
		t.Fatalf("TBPM appears %d times", count)
	}
}

func countSubstr(data []byte, needle []byte) int {
	var count int
	for i := 0; i+len(needle) <= len(data); i++ {
		if string(data[i:i+len(needle)]) == string(needle) {
			count++
		}
	}
	return count
}
