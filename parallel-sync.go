package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type fileInfo struct {
	Mtime time.Time
	Atime time.Time
	UID   int
	GID   int
	Mode  os.FileMode
}

type copyTask struct {
	From    string
	To      string
	Bailout bool
}

var chunkSize int
var src, dst string
var wg sync.WaitGroup
var copyThreads int
var copyTasks chan (copyTask)

func main() {
	chunkSize = 4 * 1024
	copyThreads = 1

	src = os.Args[1]
	dst = os.Args[2]

	copyTasks = make(chan copyTask, copyThreads*10)

	for i := 0; i < copyThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startCopyThread(copyTasks)
		}()
	}

	_ = filepath.Walk(src, visitPath)
	for i := 0; i < copyThreads; i++ {
		copyTasks <- copyTask{Bailout: true}
	}
	wg.Wait()
}

func startCopyThread(copyTasks chan copyTask) {
	var task copyTask
	var srcPath, dstPath string
	for {
		select {
		case task = <-copyTasks:
			if task.Bailout {
				return
			}
			srcPath = task.From
			dstPath = task.To
			log.Printf("Copying %s to %s", srcPath, dstPath)
			copyFileData(srcPath, dstPath)
			copyFileMetadata(srcPath, dstPath)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func visitPath(srcPath string, srcInfo os.FileInfo, err error) error {
	if err != nil {
		log.Fatalf("%s", err)
		return nil
	}
	log.Printf("Checking: %s\n", srcPath)
	verifyPath(srcPath, srcInfo, err)
	return nil
}

func verifyPath(srcPath string, srcInfo os.FileInfo, err error) {
	dstPath := strings.Replace(srcPath, src, dst, 1)
	dstInfo, dstInfoErr := os.Lstat(dstPath)

	if dstInfoErr == nil {
		if srcInfo.Mode() != dstInfo.Mode() {
			log.Printf("Removing destination %s, different type from source...", dstPath)
			os.RemoveAll(dstPath)
		}
	}

	if srcInfo.IsDir() {
		if dstInfoErr != nil {
			log.Printf("Creating directory: %s", dstPath)
			os.MkdirAll(dstPath, srcInfo.Mode())
			copyFileMetadata(srcPath, dstPath)
		} else {
			if !equalMetadata(srcPath, dstPath) {
				copyFileMetadata(srcPath, dstPath)
			}
		}
	} else {
		if srcInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
			srcSymlink, _ := os.Readlink(srcPath)
			if dstInfoErr != nil {
				log.Printf("Creating symlink: %s", dstPath)
				os.Symlink(strings.Replace(srcSymlink, src, dst, 1), dstPath)
			} else {
				dstSymlink, _ := os.Readlink(dstPath)
				if dstSymlink != strings.Replace(srcSymlink, src, dst, 1) {
					log.Printf("Replacing symlink: %s", dstPath)
					os.Remove(dstPath)
					os.Symlink(strings.Replace(srcSymlink, src, dst, 1), dstPath)
				}
			}
		} else {
			if !equalSize(srcPath, dstPath) {
				copyTasks <- copyTask{From: srcPath, To: dstPath}
			} else {
				if !equalMetadata(srcPath, dstPath) {
					copyFileMetadata(srcPath, dstPath)
				}
			}
		}
	}
}

func copyFileMetadata(src string, dst string) {
	log.Printf("Copying metadata %s to %s", src, dst)
	finfo, _ := getFileMetadata(src)
	_ = setFileMetadata(dst, finfo)
}

func copyFileData(src string, dst string) {
	log.Printf("Copying file %s to %s...", src, dst)

	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}

func compareFile(file1 string, file2 string, shallow bool) bool {
	if shallow {
		return equalMetadata(file1, file2)
	}
	return equalMetadata(file1, file2) && equalData(file1, file2)
}

func getFileMetadata(filename string) (fileInfo, error) {
	statFile, errFile := os.Stat(filename)

	if os.IsNotExist(errFile) {
		return fileInfo{}, errFile
	}
	sysStatFile := statFile.Sys().(*syscall.Stat_t)

	mtime := statFile.ModTime()
	atime := time.Unix(sysStatFile.Atim.Sec, sysStatFile.Atim.Nsec)

	uid := int(sysStatFile.Uid)
	gid := int(sysStatFile.Gid)
	mode := statFile.Mode()

	return fileInfo{mtime, atime, uid, gid, mode}, nil
}

func setFileMetadata(filename string, finfo fileInfo) error {
	mtime := finfo.Mtime
	atime := finfo.Atime
	uid := finfo.UID
	gid := finfo.GID
	mode := finfo.Mode

	err := os.Chtimes(filename, atime, mtime)
	if err != nil {
		return err
	}

	err = os.Chown(filename, uid, gid)
	if err != nil {
		return err
	}

	err = os.Chmod(filename, mode)
	if err != nil {
		return err
	}

	return nil
}

func equalMetadata(file1 string, file2 string) bool {
	if file1 == file2 {
		return true
	}

	_, errFile1 := os.Stat(file1)
	_, errFile2 := os.Stat(file2)

	if os.IsNotExist(errFile1) {
		return os.IsNotExist(errFile2)
	} else if os.IsNotExist(errFile2) {
		return false
	}

	finfo1, _ := getFileMetadata(file1)
	finfo2, _ := getFileMetadata(file2)

	if finfo1.Mtime != finfo2.Mtime || finfo1.Atime != finfo2.Atime || finfo1.UID != finfo2.UID || finfo1.GID != finfo2.GID || finfo1.Mode != finfo2.Mode {
		return false
	}

	return true
}

func equalSize(file1 string, file2 string) bool {
	if file1 == file2 {
		return true
	}

	stat1, errFile1 := os.Stat(file1)
	stat2, errFile2 := os.Stat(file2)

	if os.IsNotExist(errFile1) {
		return os.IsNotExist(errFile2)
	} else if os.IsNotExist(errFile2) {
		return false
	}

	if stat1.Size() != stat2.Size() {
		return false
	}

	return true
}

func equalData(file1 string, file2 string) bool {
	f1, err := os.Open(file1)
	if err != nil {
		log.Fatal(err)
	}

	f2, err := os.Open(file2)
	if err != nil {
		log.Fatal(err)
	}

	for {
		b1 := make([]byte, chunkSize)
		_, err1 := f1.Read(b1)

		b2 := make([]byte, chunkSize)
		_, err2 := f2.Read(b2)

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return true
			} else if err1 == io.EOF || err2 == io.EOF {
				return false
			} else {
				log.Fatal(err1, err2)
			}
		}

		if !bytes.Equal(b1, b2) {
			return false
		}
	}
}
