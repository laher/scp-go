package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)


func scpFromRemote(srcUser, srcHost, srcFile, dstFile string, options ScpOptions) error {
	dstFileInfo, err := os.Stat(dstFile)
	dstDir := dstFile
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		} else {
			//OK - create
		}
	} else if dstFileInfo.IsDir() {
		//ok - use name of srcFile
		//dstFile = filepath.Join(dstFile, filepath.Base(srcFile))
		dstDir = dstFile
	} else {
		dstDir = filepath.Dir(dstFile)
	}
	//from-scp
	session, err := connect(srcUser, srcHost, *options.Port)
	if err != nil {
		return err
	}
	defer session.Close()
	ce := make(chan error)
	go func() {
		cw, err := session.StdinPipe()
		if err != nil {
			println(err.Error())
			ce <- err
			return
		}
		defer cw.Close()
		println("Sending null byte")
		err = sendByte(cw, 0)
		if err != nil {
			println("Write error: " + err.Error())
			ce <- err
			return
		}
		r, err := session.StdoutPipe()
		if err != nil {
			println("session stdout err: " + err.Error())
			ce <- err
			return
		}
		//defer r.Close()
		scanner := bufio.NewScanner(r)
		more := true
		for more {
			scanner.Scan()
			err = scanner.Err()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error reading standard input:", err)
				ce <- err
				return
			}
			//first line
			cmdFull := scanner.Text()
			//first char
			cmd := cmdFull[0]
			//remainder, split by spaces
			parts := strings.SplitN(cmdFull[1:], " ", 3)
			fmt.Printf("Received command: %s (%v). Details: %v\n", string(cmd), cmd, cmdFull[1:])
			if cmd == 0x0 {
				//continue
				fmt.Printf("Received OK: %s\n", cmdFull[1:])
				err = sendByte(cw, 0)
				if err != nil {
					println("Write error: " + err.Error())
					ce <- err
					return
				}
			} else if cmd == 0x1 {
				fmt.Printf("Received error: %s\n", cmdFull[2:])
				ce <- errors.New(cmdFull[2:])
				return
			} else if cmd == 'D' || cmd == 'C' {
				mode, err := strconv.ParseInt(parts[0], 8, 32)
				if err != nil {
					println("Format error: " + err.Error())
					ce <- err
					return
				}
				size, err := strconv.Atoi(parts[1])
				if err != nil {
					println("Format error: " + err.Error())
					ce <- err
					return
				}
				filename := parts[2]
				fmt.Printf("Mode: %d, size: %d, filename: %s\n", mode, size, filename)
				err = sendByte(cw, 0)
				if err != nil {
					println("Write error: " + err.Error())
					ce <- err
					return
				}
				if cmd == 'C' {
					thisDstFile := filepath.Join(dstDir, filename)
					println("Creating destination file: ", thisDstFile)
					//TODO: mode here
					fw, err := os.Create(thisDstFile)
					if err != nil {
						ce <- err
						println("File creation error: " + err.Error())
						return
					}
					defer fw.Close()

					//todo - buffer ...
					b := make([]byte, size)
					_, err = r.Read(b)
					if err != nil {
						println("Read error: " + err.Error())
						ce <- err
						return
					}
					_, err = fw.Write(b)
					if err != nil {
						println("Write error: " + err.Error())
						ce <- err
						return
					}
					err = fw.Close()
					if err != nil {
						println(err.Error())
						ce <- err
						return
					}
					_, err = cw.Write([]byte{0})
					if err != nil {
						println("Write error: " + err.Error())
						ce <- err
						return
					}
					err = cw.Close()
					if err != nil {
						println(err.Error())
						ce <- err
						return
					}
				} else { //D command (directory)
					thisDstFile := filepath.Join(dstDir, filename)
					fileMode := os.FileMode(uint32(mode))
					err = os.Mkdir(thisDstFile, fileMode)
					if err != nil {
						println("Mkdir error: " + err.Error())
						os.Exit(1)
						ce <- err
						return
					}
					dstDir = thisDstFile
				}
			} else if cmd == 'E' { //E command: go back out of dir
				dstDir = filepath.Dir(dstDir)
/*
				err = sendByte(cw, 0)
				if err != nil {
					println("Write error: " + err.Error())
					ce <- err
					return
				}
*/
			} else {
				fmt.Printf("Command '%v' NOT implemented\n", cmd)
				return
			}
		}
	}()
	remoteOpts := "-qf";
	if *options.IsRecursive {
		remoteOpts += "r"
	}
	err = session.Run("/usr/bin/scp "+remoteOpts+" " + srcFile)
	if err != nil {
		println("Failed to run remote scp: " + err.Error())
	}
	return err

}

