package zip

import (
	"os"
	"testing"
)

func TestCreateArchiveFromDir(t *testing.T) {
	t.Parallel()
	const expectedArchivePath = "testdata/dir.tar.bz2"

	defer func() {
		if err := os.RemoveAll(expectedArchivePath); err != nil {
			t.Logf("failed to delete dir: %v", err)
		}
	}()

	if err := CreateArchiveFromDir("testdata/dir"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(expectedArchivePath); err != nil {
		t.Fatalf("failed to locate the archive file: %v", err)
	}
}

func TestCreateArchiveFromFile(t *testing.T) {
	t.Parallel()
	const expectedArchivePath = "testdata/file.txt.tar.bz2"

	defer func() {
		if err := os.Remove(expectedArchivePath); err != nil {
			t.Logf("failed to delete dir: %v", err)
		}
	}()

	if err := CreateArchiveFromDir("testdata/file.txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(expectedArchivePath); err != nil {
		t.Fatalf("failed to locate the archive file: %v", err)
	}
}
