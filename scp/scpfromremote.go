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
	"time"
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
	session, err := connect(srcUser, srcHost, options.Port, options.IsCheckKnownHosts)
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
			fmt.Fprintln(os.Stderr, "session stdout err: " + err.Error() + " continue anyway")
			ce <- err
			return
		}
		if options.IsVerbose {
			fmt.Fprintln(os.Stderr, "Sending null byte")
		}
		err = sendByte(cw, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Write error: " + err.Error())
			ce <- err
			return
		}
		//defer r.Close()
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
			if cmd == 0x0 {
				//continue
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Received OK \n")
				}
		/*		err = sendByte(cw, 0)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Write error: " + err.Error())
					ce <- err
					return
				}
*/
			} else if cmd == 'E' { //E command: go back out of dir
				dstDir = filepath.Dir(dstDir)
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Received End-Dir\n")
				}
				err = sendByte(cw, 0)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Write error: " + err.Error())
					ce <- err
					return
				}

			} else if cmd == 0xA { //0xA command: end?
				if options.IsVerbose {
					fmt.Fprintf(os.Stderr, "Received All-done\n")
				}

				err = sendByte(cw, 0)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Write error: " + err.Error())
					ce <- err
					return
				}

				return
			} else {
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

				if cmd == 0x1 {
					fmt.Fprintf(os.Stderr, "Received error message: %s\n", cmdFull[1:])
					ce <- errors.New(cmdFull[1:])
					return
				} else if cmd == 'D' || cmd == 'C' {
					mode, err := strconv.ParseInt(parts[0], 8, 32)
					if err != nil {
						fmt.Fprintln(os.Stderr, "Format error: " + err.Error())
						ce <- err
						return
					}
					size, err := strconv.Atoi(parts[1])
					if err != nil {
						fmt.Fprintln(os.Stderr, "Format error: " + err.Error())
						ce <- err
						return
					}
					filename := parts[2]
					if options.IsVerbose {
						fmt.Fprintf(os.Stderr, "Mode: %d, size: %d, filename: %s\n", mode, size, filename)
					}
					err = sendByte(cw, 0)
					if err != nil {
						fmt.Fprintln(os.Stderr, "Send error: " + err.Error())
						ce <- err
						return
					}
					if cmd == 'C' {
						thisDstFile := filepath.Join(dstDir, filename)
						if options.IsVerbose {
							fmt.Fprintln(os.Stderr, "Creating destination file: ", thisDstFile)
						}
						format := "\r%s\t\t%d%%\t%dkb\t%0.2fkb/s\t%v"
						startTime := time.Now()
						percent := int64(0)
						spd := float64(0)
						totTime := startTime.Sub(startTime)
						tot := int64(0)
						fmt.Printf(format, thisDstFile, percent, tot, spd, totTime)

						//TODO: mode here
						fw, err := os.Create(thisDstFile)
						if err != nil {
							ce <- err
							fmt.Fprintln(os.Stderr, "File creation error: " + err.Error())
							return
						}
						defer fw.Close()

						//todo - buffer ...
						b := make([]byte, size)
						_, err = r.Read(b)
						if err != nil {
							fmt.Fprintln(os.Stderr, "Read error: " + err.Error())
							ce <- err
							return
						}
						_, err = fw.Write(b)
						if err != nil {
							fmt.Fprintln(os.Stderr, "Write error: " + err.Error())
							ce <- err
							return
						}
						err = fw.Close()
						if err != nil {
							println(err.Error())
							ce <- err
							return
						}
						nb := make([]byte, 1)
						_, err = r.Read(nb)
						if err != nil {
							println(err.Error())
							ce <- err
							return
						}
						_, err = cw.Write([]byte{0})
						if err != nil {
							fmt.Fprintln(os.Stderr, "Send null-byte error: " + err.Error())
							ce <- err
							return
						}
						tot = int64(size)
						percent = (100 * tot) / int64(size)
						nowTime := time.Now()
						totTime = nowTime.Sub(startTime)
						spd = float64(tot/1000) / totTime.Seconds()
						fmt.Printf(format, thisDstFile, percent, size, spd, totTime)
						fmt.Println()

					} else { //D command (directory)
						thisDstFile := filepath.Join(dstDir, filename)
						fileMode := os.FileMode(uint32(mode))
						err = os.MkdirAll(thisDstFile, fileMode)
						if err != nil {
							fmt.Fprintln(os.Stderr, "Mkdir error: " + err.Error())
							os.Exit(1)
							ce <- err
							return
						}
						dstDir = thisDstFile
					}
				} else {
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
		fmt.Fprintln(os.Stderr, "Failed to run remote scp: " + err.Error())
	}
	return err

}
