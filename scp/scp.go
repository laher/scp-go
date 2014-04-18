package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"errors"
	"fmt"
	"github.com/laher/uggo"
	"io"
	"os"
	"strings"
)

const (
	VERSION = "0.4.0"
)

type SecureCopier struct {
	Port              int
	IsRecursive       bool
	IsRemoteTo        bool
	IsRemoteFrom      bool
	IsQuiet           bool
	IsVerbose         bool
	IsCheckKnownHosts bool
	KeyFile        string

	srcHost string
	srcUser string
	srcFile string
	dstHost string
	dstUser string
	dstFile string
}
func (scp *SecureCopier) Name() string {
	return "scp"
}

//func Scp(call []string) error {
func (scp *SecureCopier) ParseFlags(call []string, errPipe io.Writer) (error, int) {
	//fmt.Fprintf(errPipe, "Warning: this scp is incomplete and not currently working with all ssh servers\n")
	flagSet := uggo.NewFlagSetDefault("scp", "[options] [[user@]host1:]file1 [[user@]host2:]file2", VERSION)
	flagSet.BoolVar(&scp.IsRecursive, "r", false, "Recursive copy")
	flagSet.IntVar(&scp.Port, "P", 22, "Port number")
	flagSet.BoolVar(&scp.IsRemoteTo, "t", false, "Remote 'to' mode - not currently supported")
	flagSet.BoolVar(&scp.IsRemoteFrom, "f", false, "Remote 'from' mode - not currently supported")
	flagSet.BoolVar(&scp.IsQuiet, "q", false, "Quiet mode: disables the progress meter as well as warning and diagnostic messages")
	flagSet.BoolVar(&scp.IsVerbose, "v", false, "Verbose mode - output differs from normal scp")
	flagSet.BoolVar(&scp.IsCheckKnownHosts, "check-known-hosts", false, "Check known hosts - experimental!")
	flagSet.StringVar(&scp.KeyFile, "key-file", "", "Use this keyfile to authenticate")
	err, code := flagSet.ParsePlus(call[1:])
	if err != nil {
		return err, code
	}
	if scp.IsRemoteTo || scp.IsRemoteFrom {
		return errors.New("This scp does NOT implement 'remote-remote scp'. Yet."), 1
	}
	args := flagSet.Args()
	if len(args) != 2 {
		flagSet.Usage()
		return errors.New("Not enough args"), 1
	}

	scp.srcFile, scp.srcHost, scp.srcUser, err = parseTarget(args[0])
	if err != nil {
		fmt.Fprintln(errPipe, "Error parsing source")
		return err, 1
	}
	scp.dstFile, scp.dstHost, scp.dstUser, err = parseTarget(args[1])
	if err != nil {
		fmt.Fprintln(errPipe, "Error parsing destination")
		return err, 1
	}
	return nil, 0
}

func (scp *SecureCopier) Exec(inPipe io.Reader, outPipe io.Writer, errPipe io.Writer) (error, int) {
	if scp.srcHost != "" && scp.dstHost != "" {
		return errors.New("remote->remote NOT implemented (yet)!"), 1
	} else if scp.srcHost != "" {
		err := scp.scpFromRemote(scp.srcUser, scp.srcHost, scp.srcFile, scp.dstFile, inPipe, outPipe, errPipe)
		if err != nil {
			fmt.Fprintln(errPipe, errPipe, "Failed to run 'from-remote' scp: "+err.Error())
			return err, 1
		}
		return nil, 0

	} else if scp.dstHost != "" {
		err := scp.scpToRemote(scp.srcFile, scp.dstUser, scp.dstHost, scp.dstFile, outPipe, errPipe)
		if err != nil {
			fmt.Fprintln(errPipe, "Failed to run 'to-remote' scp: "+err.Error())
			return err, 1
		}
		return nil, 0
	} else {
		srcReader, err := os.Open(scp.srcFile)
		defer srcReader.Close()
		if err != nil {
			fmt.Fprintln(errPipe, "Failed to open local source file ('local-local' scp): "+err.Error())
			return err, 1
		}
		dstWriter, err := os.OpenFile(scp.dstFile, os.O_CREATE|os.O_WRONLY, 0777)
		defer dstWriter.Close()
		if err != nil {
			fmt.Fprintln(errPipe, "Failed to open local destination file ('local-local' scp): "+err.Error())
			return err, 1
		}
		n, err := io.Copy(dstWriter, srcReader)
		fmt.Fprintf(errPipe, "wrote %d bytes\n", n)
		if err != nil {
			fmt.Fprintln(errPipe, "Failed to run 'local-local' copy: "+err.Error())
			return err, 1
		}
		err = dstWriter.Close()
		if err != nil {
			fmt.Fprintln(errPipe, "Failed to close local destination: "+err.Error())
			return err, 1
		}

	}
	return nil, 0
}

//TODO: error for multiple ats or multiple colons
func parseTarget(target string) (string, string, string, error) {
	//treat windows drive refs as local
	if strings.Contains(target, ":\\") {
		if strings.Index(target, ":\\") == 1 {
			return target, "", "", nil
		}
	}
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


func sendByte(w io.Writer, val byte) error {
	_, err := w.Write([]byte{val})
	return err
}

