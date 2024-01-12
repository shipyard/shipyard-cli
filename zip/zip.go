package zip

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"

	"github.com/dsnet/compress/bzip2"
)

const archiveExtension = ".tar.bz2"

func CreateArchiveFromDir(source string) error {
	return createArchive(source, func(tarWriter *tar.Writer) error {
		return filepath.Walk(source, func(file string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return writeToArchive(tarWriter, file, fi)
		})
	})
}

func CreateArchiveFromFile(name string) error {
	fi, err := os.Stat(name)
	if err != nil {
		return err
	}
	return createArchive(fi.Name(), func(tarWriter *tar.Writer) error {
		return writeToArchive(tarWriter, name, fi)
	})
}

func writeToArchive(tarWriter *tar.Writer, fileName string, fileInfo os.FileInfo) error {
	if !fileInfo.Mode().IsRegular() {
		return nil
	}
	header, err := tar.FileInfoHeader(fileInfo, fileName)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(fileName)
	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}
	file, err := os.Open(fileName)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(tarWriter, file)
	return err
}

func createArchive(targetName string, writeFunc func(*tar.Writer) error) error {
	tarFile, err := os.Create(targetName + archiveExtension)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	bz2Writer, err := bzip2.NewWriter(tarFile, &bzip2.WriterConfig{Level: bzip2.BestCompression})
	if err != nil {
		return err
	}
	defer bz2Writer.Close()

	tarWriter := tar.NewWriter(bz2Writer)
	defer tarWriter.Close()

	return writeFunc(tarWriter)
}
