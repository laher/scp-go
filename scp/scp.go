package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"flag"
	"fmt"
	"github.com/howeyc/gopass"
	"github.com/laher/uggo"
	"io"
	"os"
	"os/user"
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
	options.IsRecursive = flagSet.Bool("r", false, "Recursive copy")
	options.Port = flagSet.Int("P", 22, "Port number")
	options.IsRemoteTo = flagSet.Bool("t", false, "Remote 'to' mode - not currently supported")
	options.IsRemoteFrom = flagSet.Bool("f", false, "Remote 'from' mode - not currently supported")
	helpFlag := flagSet.Bool("help", false, "Show this help")
	err := flagSet.Parse(uggo.Gnuify(call[1:]))
	if err != nil {
		println("Error parsing flags")
		return err
	}
	if *options.IsRecursive {
		//return errors.New("This scp does NOT implement 'recursive scp'. Yet.")
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
	} else {
		srcReader, err := os.Open(srcFile)
		defer srcReader.Close()
		if err != nil {
			println("Failed to open local source file ('local-local' scp): " + err.Error())
			return err
		}
		dstWriter, err := os.OpenFile(dstFile, os.O_CREATE | os.O_WRONLY, 0777)
		defer dstWriter.Close()
		if err != nil {
			println("Failed to open local destination file ('local-local' scp): " + err.Error())
			return err
		}
		n, err := io.Copy(dstWriter, srcReader)
		fmt.Printf("wrote %d bytes\n", n)
		if err != nil {
			println("Failed to run 'local-local' copy: " + err.Error())
			return err
		}
		err = dstWriter.Close()
		return err
	}
	return nil
}
func sendByte(w io.Writer, val byte) error {
	_, err := w.Write([]byte{val})
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
