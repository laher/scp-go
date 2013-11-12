package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"bufio"
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"flag"
	"fmt"
	"github.com/howeyc/gopass"
	"github.com/laher/uggo"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type ScpOptions struct {
	Port         *int
	IsRecursive  *bool
	IsRemoteTo   *bool
	IsRemoteFrom *bool
}

type clientPassword string

func (p clientPassword) Password(user string) (string, error) {
	return string(p), nil
}

//TODO: error for multiple ats or multiple colons
func parseTarget(target string) (string, string, string, error) {
	if strings.Contains(target, ":") {
		//remote
		parts := strings.Split(target, ":")
		userHost := parts[0]
		file := parts[1]
		user := ""
		var host string
		if strings.Contains(userHost, "@") {
			uhParts := strings.Split(userHost, "@")
			user = uhParts[0]
			host = uhParts[1]
		} else {
			host = userHost
		}
		return file, host, user, nil
	} else {
		//local
		return target, "", "", nil
	}
}

func Scp(call []string) error {
	options := ScpOptions{}
	flagSet := flag.NewFlagSet("scp", flag.ContinueOnError)
	options.IsRecursive = flagSet.Bool("r", false, "TODO - Recursive copy")
	options.Port = flagSet.Int("P", 22, "Port number")
	options.IsRemoteTo = flagSet.Bool("t", false, "Not supported")
	options.IsRemoteFrom = flagSet.Bool("f", false, "Not supported")
	helpFlag := flagSet.Bool("help", false, "Show this help")
	err := flagSet.Parse(uggo.Gnuify(call[1:]))
	if err != nil {
		println("Error parsing flags")
		return err
	}
	if *options.IsRecursive {
		return errors.New("This scp does NOT implement 'recursive scp'. Yet.")
	}
	if *options.IsRemoteTo || *options.IsRemoteFrom {
		return errors.New("This scp does NOT implement 'remote scp'. Yet.")
	}
	args := flagSet.Args()
	if *helpFlag || len(args) != 2 {
		println("`scp` [options] [[user@]host1:]file1 [[user@]host2:]file2")
		flagSet.PrintDefaults()
		return nil
	}

	srcFile, srcHost, srcUser, err := parseTarget(args[0])
	if err != nil {
		println("Error parsing source")
		return err
	}
	dstFile, dstHost, dstUser, err := parseTarget(args[1])
	if err != nil {
		println("Error parsing destination")
		return err
	}
	if srcHost != "" && dstHost != "" {
		return errors.New("remote->remote NOT implemented (yet)!")
	} else if srcHost != "" {
		err = scpFromRemote(srcUser, srcHost, srcFile, dstFile, options)
		if err != nil {
			println("Failed to run 'from-remote' scp: " + err.Error())
		}
		return err

	} else if dstHost != "" {
		err = scpToRemote(srcFile, dstUser, dstHost, dstFile, options)
		if err != nil {
			println("Failed to run 'to-remote' scp: " + err.Error())
		}
		return err
	}
	return nil
}
func sendByte(w io.Writer, val byte) error {
	_, err := w.Write([]byte{val})
	return err
}
func scpFromRemote(srcUser, srcHost, srcFile, dstFile string, options ScpOptions) error {
	dstFileInfo, err := os.Stat(dstFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		} else {
			//OK - create
		}
	} else if dstFileInfo.IsDir() {
		//ok - use name of srcFile
		dstFile = filepath.Join(dstFile, filepath.Base(srcFile))
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
		println("Creating destination file ", dstFile)
		fw, err := os.Create(dstFile)
		if err != nil {
			ce <- err
			println("File creation error: " + err.Error())
			return
		}
		defer fw.Close()
		scanner := bufio.NewScanner(r)
		scanner.Scan()
		err = scanner.Err()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading standard input:", err)
			ce <- err
			return
		}
		cmdFull := scanner.Text()
		cmd := string(cmdFull[0])
		parts := strings.Split(cmdFull[1:], " ")
		fmt.Printf("Received command: %s. Details: %v\n", cmd, parts)
		if cmd == "C" {
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
			base := parts[2]
			fmt.Printf("Mode: %d, size: %d, file: %s\n", mode, size, base)
			err = sendByte(cw, 0)
			if err != nil {
				println("Write error: " + err.Error())
				ce <- err
				return
			}
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
			//TODO: chmod here
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
		} else {
			fmt.Printf("Command '%s' NOT implemented\n", cmd)
			return
		}
	}()
	err = session.Run("/usr/bin/scp -qrf " + srcFile)
	if err != nil {
		println("Failed to run remote scp: " + err.Error())
	}
	return err

}

//to-scp
func scpToRemote(srcFile, dstUser, dstHost, dstFile string, options ScpOptions) error {
	srcFileInfo, err := os.Stat(srcFile)
	if err != nil {
		return err
	}
	session, err := connect(dstUser, dstHost, *options.Port)
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
		fileReader, err := os.Open(srcFile)
		if err != nil {
			ce <- err
			println(err.Error())
			return
		}
		defer fileReader.Close()
		header := fmt.Sprintf("%s %d %s\n", "C0644", srcFileInfo.Size(), dstFile)
		fmt.Printf("Sending: %s\n", header)
		procWriter.Write([]byte(header))
		io.Copy(procWriter, fileReader)
		// terminate with null byte
		err = sendByte(procWriter, 0)
		fmt.Println("Sent file plus null-byte.")
		err = procWriter.Close()
		if err != nil {
			println(err.Error())
			ce <- err
			return
		}
		err = fileReader.Close()
		if err != nil {
			println(err.Error())
			ce <- err
			return
		}
	}()
	err = session.Run("/usr/bin/scp -qrt ./")
	if err != nil {
		println("Failed to run remote scp: " + err.Error())
	}
	return err
}

func connect(userName, host string, port int) (*ssh.Session, error) {
	if userName == "" {
		u, err := user.Current()
		userName = u.Username
		if err != nil {
			return nil, err
		}
	}
	fmt.Printf("%s@%s's password:", userName, host)
	pass := gopass.GetPasswd()
	password := clientPassword(pass)
	clientConfig := &ssh.ClientConfig{
		User: userName,
		Auth: []ssh.ClientAuth{
			ssh.ClientAuthPassword(password),
		},
	}
	target := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", target, clientConfig)
	if err != nil {
		println("Failed to dial: " + err.Error())
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		println("Failed to create session: " + err.Error())
	} else {
		println("Got session")
	}
	return session, err

}
