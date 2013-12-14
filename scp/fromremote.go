package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"bufio"
	"errors"
	"fmt"
	"io"
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
			//OK - create file/dir
		}
	} else if dstFileInfo.IsDir() {
		//ok - use name of srcFile
		//dstFile = filepath.Join(dstFile, filepath.Base(srcFile))
		dstDir = dstFile
	} else {
		dstDir = filepath.Dir(dstFile)
	}
	//from-scp
	session, err := connect(srcUser, srcHost, options.Port, options.IsCheckKnownHosts, options.IsVerbose)
	if err != nil {
		return err
	} else if options.IsVerbose {
		fmt.Fprintln(os.Stderr, "Got session")
	}
	defer session.Close()
	ce := make(chan error)
	go func() {
		cw, err := session.StdinPipe()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			ce <- err
			return
		}
		defer cw.Close()
		r, err := session.StdoutPipe()
		if err != nil {
			fmt.Fprintln(os.Stderr, "session stdout err: "+err.Error()+" continue anyway")
			ce <- err
			return
		}
		if options.IsVerbose {
			fmt.Fprintln(os.Stderr, "Sending null byte")
		}
		err = sendByte(cw, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Write error: "+err.Error())
			ce <- err
			return
		}
		//defer r.Close()
		//use a scanner for processing individual commands, but not files themselves
		scanner := bufio.NewScanner(r)
		more := true
		for more {
			cmdArr := make([]byte, 1)
			n, err := r.Read(cmdArr)
			if err != nil {
				if err == io.EOF {
					//no problem.
					if options.IsVerbose {
						fmt.Fprintln(os.Stderr, "Received EOF from remote server")
					}
				} else {
					fmt.Fprintln(os.Stderr, "Error reading standard input:", err)
					ce <- err
				}
				return
			}
			if n < 1 {
				fmt.Fprintln(os.Stderr, "Error reading next byte from standard input")
				ce <- errors.New("Error reading next byte from standard input")
				return
			}
			cmd := cmdArr[0]
			if options.IsVerbose {
				fmt.Fprintf(os.Stderr, "Sink: %s (%v)\n", string(cmd), cmd)
			}
			switch cmd {
			case 0x0:
			//if cmd == 0x0 {
				//continue
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Received OK \n")
				}
			case 'E':
			//} else if cmd == 'E' { 
			//E command: go back out of dir
				dstDir = filepath.Dir(dstDir)
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Received End-Dir\n")
				}
				err = sendByte(cw, 0)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Write error: %s", err.Error())
					ce <- err
					return
				}
			case 0xA:
			//} else if cmd == 0xA { 
			//0xA command: end?
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Received All-done\n")
				}

				err = sendByte(cw, 0)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Write error: "+err.Error())
					ce <- err
					return
				}

				return
			default:
				scanner.Scan()
				err = scanner.Err()
				if err != nil {
					if err == io.EOF {
						//no problem.
						if options.IsVerbose {
							fmt.Fprintln(os.Stderr, "Received EOF from remote server")
						}
					} else {
						fmt.Fprintln(os.Stderr, "Error reading standard input:", err)
						ce <- err
					}
					return
				}
				//first line
				cmdFull := scanner.Text()
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Details: %v\n", cmdFull)
				}
				//remainder, split by spaces
				parts := strings.SplitN(cmdFull, " ", 3)

				switch cmd {
				case 0x1:
					fmt.Fprintf(os.Stderr, "Received error message: %s\n", cmdFull[1:])
					ce <- errors.New(cmdFull[1:])
					return
				case 'D','C':
					mode, err := strconv.ParseInt(parts[0], 8, 32)
					if err != nil {
						fmt.Fprintln(os.Stderr, "Format error: "+err.Error())
						ce <- err
						return
					}
					sizeUint, err := strconv.ParseUint(parts[1], 10, 64)
					size := int64(sizeUint)
					if err != nil {
						fmt.Fprintln(os.Stderr, "Format error: "+err.Error())
						ce <- err
						return
					}
					filename := parts[2]
					if options.IsVerbose {
						fmt.Fprintf(os.Stderr, "Mode: %d, size: %d, filename: %s\n", mode, size, filename)
					}
					err = sendByte(cw, 0)
					if err != nil {
						fmt.Fprintln(os.Stderr, "Send error: "+err.Error())
						ce <- err
						return
					}
					if cmd == 'C' {
						//C command - file
						thisDstFile := filepath.Join(dstDir, filename)
						if options.IsVerbose {
							fmt.Fprintln(os.Stderr, "Creating destination file: ", thisDstFile)
						}
						tot := int64(0)
						pb := NewProgressBar(filename, size)
						pb.Update(0)

						//TODO: mode here
						fw, err := os.Create(thisDstFile)
						if err != nil {
							ce <- err
							fmt.Fprintln(os.Stderr, "File creation error: "+err.Error())
							return
						}
						defer fw.Close()

						//buffered by 4096 bytes
						bufferSize := int64(4096)
						lastPercent := int64(0)
						for tot < size {
							if bufferSize > size-tot {
								bufferSize = size - tot
							}
							b := make([]byte, bufferSize)
							n, err = r.Read(b)
							if err != nil {
								fmt.Fprintln(os.Stderr, "Read error: "+err.Error())
								ce <- err
								return
							}
							tot += int64(n)
							//write to file
							_, err = fw.Write(b[:n])
							if err != nil {
								fmt.Fprintln(os.Stderr, "Write error: "+err.Error())
								ce <- err
								return
							}
							percent := (100 * tot) / size
							if percent > lastPercent {
								pb.Update(tot)
							}
							lastPercent = percent
						}
						//close file writer & check error
						err = fw.Close()
						if err != nil {
							fmt.Fprintln(os.Stderr, err.Error())
							ce <- err
							return
						}
						//get next byte from channel reader
						nb := make([]byte, 1)
						_, err = r.Read(nb)
						if err != nil {
							println(err.Error())
							ce <- err
							return
						}
						//TODO check value received in nb
						//send null-byte back
						_, err = cw.Write([]byte{0})
						if err != nil {
							fmt.Fprintln(os.Stderr, "Send null-byte error: "+err.Error())
							ce <- err
							return
						}
						pb.Update(tot)
						fmt.Println() //new line
					} else { 
						//D command (directory)
						thisDstFile := filepath.Join(dstDir, filename)
						fileMode := os.FileMode(uint32(mode))
						err = os.MkdirAll(thisDstFile, fileMode)
						if err != nil {
							fmt.Fprintln(os.Stderr, "Mkdir error: "+err.Error())
							os.Exit(1)
							ce <- err
							return
						}
						dstDir = thisDstFile
					}
				default:
					fmt.Fprintf(os.Stderr, "Command '%v' NOT implemented\n", cmd)
					return
				}
			}
		}
		err = cw.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error closing process writer: ", err.Error())
			ce <- err
			return
		}
	}()
	remoteOpts := "-f"
	if options.IsQuiet {
		remoteOpts += "q"
	}
	if options.IsRecursive {
		remoteOpts += "r"
	}
	err = session.Run("/usr/bin/scp " + remoteOpts + " " + srcFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to run remote scp: "+err.Error())
	}
	return err

}
