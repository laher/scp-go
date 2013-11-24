package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)


func processDir(procWriter io.Writer, srcFilePath string, srcFileInfo os.FileInfo, options ScpOptions) error {
	err := sendDir(procWriter, srcFilePath, srcFileInfo, options)
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
			err = processDir(procWriter, filepath.Join(srcFilePath,fi.Name()), fi, options)
			if err != nil {
				return err
			}
		} else {
			err = sendFile(procWriter, filepath.Join(srcFilePath,fi.Name()), fi, options)
			if err != nil {
				return err
			}
		}
	}
	//TODO process errors
	err = sendEndDir(procWriter, options)
	return err
}

func sendEndDir(procWriter io.Writer, options ScpOptions) error {
	header := fmt.Sprintf("E\n")
	if options.IsVerbose {
		fmt.Fprintf(os.Stderr, "Sending end dir: %s", header)
	}
	_, err := procWriter.Write([]byte(header))
	return err
}

func sendDir(procWriter io.Writer, srcPath string, srcFileInfo os.FileInfo, options ScpOptions) error {
	mode := uint32(srcFileInfo.Mode().Perm())
	header := fmt.Sprintf("D%04o 0 %s\n", mode, filepath.Base(srcPath))
	if options.IsVerbose {
		fmt.Fprintf(os.Stderr, "Sending Dir header : %s", header)
	}
	_, err := procWriter.Write([]byte(header))
	return err
}

func sendFile(procWriter io.Writer, srcPath string, srcFileInfo os.FileInfo, options ScpOptions) error {
	//single file
	mode := uint32(srcFileInfo.Mode().Perm())
	fileReader, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer fileReader.Close()
	size  := srcFileInfo.Size()
	header := fmt.Sprintf("C%04o %d %s\n", mode, size, filepath.Base(srcPath))
	if options.IsVerbose {
		fmt.Fprintf(os.Stderr, "Sending File header: %s", header)
	}
	format := "\r%s\t\t%d%%\t%dkb\t%0.2fkb/s\t%v"
	startTime := time.Now()
	percent := int64(0)
	spd := float64(0)
	totTime := startTime.Sub(startTime)
	tot := int64(0)
	fmt.Printf(format, srcPath, percent, tot, spd, totTime)
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

	err = fileReader.Close()
	if options.IsVerbose {
		fmt.Fprintln(os.Stderr, "Sent file plus null-byte.")
	}
	tot = size
	percent = (100 * tot) / size
	nowTime := time.Now()
	totTime = nowTime.Sub(startTime)
	spd = float64(tot/1000) / totTime.Seconds()
	fmt.Printf(format, srcPath, percent, size, spd, totTime)
	fmt.Println()

	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	return err
}

//to-scp
func scpToRemote(srcFile, dstUser, dstHost, dstFile string, options ScpOptions) error {
	
	srcFileInfo, err := os.Stat(srcFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not stat source file "+srcFile)
		return err
	}
	session, err := connect(dstUser, dstHost, options.Port)
	if err != nil {
		return err
	} else if options.IsVerbose {
		fmt.Fprintln(os.Stderr, "Got session")
	}
	defer session.Close()
	ce := make(chan error)
	if dstFile == "" {
		dstFile = filepath.Base(srcFile)
		//dstFile = "."
	}
	go func() {
		procWriter, err := session.StdinPipe()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			ce <- err
			return
		}
		defer procWriter.Close()
		if options.IsRecursive {
			if srcFileInfo.IsDir() {
				err = processDir(procWriter, srcFile, srcFileInfo, options)
				if err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
					ce <- err
				}
			} else {
				err = sendFile(procWriter, srcFile, srcFileInfo, options)
				if err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
					ce <- err
				}
			}
		} else {
			if srcFileInfo.IsDir() {
				ce <- errors.New("Error: Not a regular file")
				return
			} else {
				err = sendFile(procWriter, srcFile, srcFileInfo, options)
				if err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
					ce <- err
				}
			}
		}
		err = procWriter.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			ce <- err
			return
		}
	}()
	go func() {
		select {
		case err, ok := <-ce:
			fmt.Fprintln(os.Stderr, "Error:", err, ok)
			os.Exit(1)
		}
	}()

	remoteOpts := "-qt";
	if options.IsRecursive {
		remoteOpts += "r"
	}
	err = session.Run("/usr/bin/scp "+remoteOpts+" "+dstFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to run remote scp: " + err.Error())
	}
	return err
}

