package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func processDir(procWriter io.Writer, srcFilePath string, srcFileInfo os.FileInfo) error {
	err := sendDir(procWriter, srcFilePath, srcFileInfo)
	if err != nil {
		return err
	}
	dir, err := os.Open(srcFilePath)
	if err != nil {
		return err
	}
	fis, err := dir.Readdir(0)
	if err != nil {
		return err
	}
	for _, fi := range fis {
		if fi.IsDir() {
			err = processDir(procWriter, filepath.Join(srcFilePath, fi.Name()), fi)
			if err != nil {
				return err
			}
		} else {
			err = sendFile(procWriter, filepath.Join(srcFilePath, fi.Name()), fi)
			if err != nil {
				return err
			}
		}
	}
	//TODO process errors
	err = sendEndDir(procWriter)
	return err
}

func sendEndDir(procWriter io.Writer) error {
	header := fmt.Sprintf("E\n")
	fmt.Printf("Sending end dir: %s", header)
	_, err := procWriter.Write([]byte(header))
	return err
}

func sendDir(procWriter io.Writer, srcPath string, srcFileInfo os.FileInfo) error {
	mode := uint32(srcFileInfo.Mode().Perm())
	header := fmt.Sprintf("D%04o 0 %s\n", mode, filepath.Base(srcPath))
	fmt.Printf("Sending Dir header : %s", header)
	_, err := procWriter.Write([]byte(header))
	return err
}

func sendFile(procWriter io.Writer, srcPath string, srcFileInfo os.FileInfo) error {
	//single file
	mode := uint32(srcFileInfo.Mode().Perm())
	fileReader, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer fileReader.Close()
	header := fmt.Sprintf("C%04o %d %s\n", mode, srcFileInfo.Size(), filepath.Base(srcPath))
	fmt.Printf("Sending File header: %s", header)
	_, err = procWriter.Write([]byte(header))
	if err != nil {
		return err
	}
	_, err = io.Copy(procWriter, fileReader)
	if err != nil {
		return err
	}
	// terminate with null byte
	err = sendByte(procWriter, 0)
	if err != nil {
		return err
	}
	fmt.Println("Sent file plus null-byte.")
	err = fileReader.Close()
	if err != nil {
		println(err.Error())
	}
	return err
}

//to-scp
func scpToRemote(srcFile, dstUser, dstHost, dstFile string, options ScpOptions) error {
	srcFileInfo, err := os.Stat(srcFile)
	if err != nil {
		return err
	}
	session, err := connect(dstUser, dstHost, options.Port)
	if err != nil {
		return err
	}
	defer session.Close()
	ce := make(chan error)
	if dstFile == "" {
		dstFile = filepath.Base(srcFile)
	}
	go func() {
		procWriter, err := session.StdinPipe()
		if err != nil {
			println(err.Error())
			ce <- err
			return
		}
		defer procWriter.Close()
		if options.IsRecursive {
			if srcFileInfo.IsDir() {
				err = processDir(procWriter, srcFile, srcFileInfo)
				if err != nil {
					println(err.Error())
					ce <- err
				}
			} else {
				err = sendFile(procWriter, srcFile, srcFileInfo)
				if err != nil {
					println(err.Error())
					ce <- err
				}
			}
		} else {
			if srcFileInfo.IsDir() {
				ce <- errors.New("Error: Not a regular file")
				return
			} else {
				err = sendFile(procWriter, srcFile, srcFileInfo)
				if err != nil {
					println(err.Error())
					ce <- err
				}
			}
		}
		err = procWriter.Close()
		if err != nil {
			println(err.Error())
			ce <- err
			return
		}
	}()
	go func() {
		select {
		case err, ok := <-ce:
			fmt.Println("Error:", err, ok)
			os.Exit(1)
		}
	}()

	remoteOpts := "-t"
	if options.IsQuiet {
		remoteOpts += "q"
	}
	if options.IsRecursive {
		remoteOpts += "r"
	}
	err = session.Run("/usr/bin/scp " + remoteOpts + " " + dstFile)
	if err != nil {
		println("Failed to run remote scp: " + err.Error())
	}
	return err
}
