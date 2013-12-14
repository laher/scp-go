package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"fmt"
	"github.com/howeyc/gopass"
	"github.com/laher/uggo"
	"io"
	"net"
	"os"
	"os/user"
	"strings"
)

const (
	VERSION = "0.3.0"
)

type ScpOptions struct {
	Port              int
	IsRecursive       bool
	IsRemoteTo        bool
	IsRemoteFrom      bool
	IsQuiet           bool
	IsVerbose         bool
	IsCheckKnownHosts bool
	KeyFile        string
}

type passwordPrompt struct {
	userName string
	host string
	password string
}

func (p passwordPrompt) Password(userName string) (string, error) {
	if userName == "" {
 
		//userName = p.userName
	} else {
		p.userName = userName
	}
	if p.password == "" {
		fmt.Printf("%s@%s's password:", p.userName, p.host)
		p.password = string(gopass.GetPasswd())
	}
	return p.password, nil
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

func Scp(call []string) error {
	//fmt.Fprintf(os.Stderr, "Warning: this scp is incomplete and not currently working with all ssh servers\n")
	options := ScpOptions{}
	flagSet := uggo.NewFlagSetDefault("scp", "[options] [[user@]host1:]file1 [[user@]host2:]file2", VERSION)
	flagSet.BoolVar(&options.IsRecursive, "r", false, "Recursive copy")
	flagSet.IntVar(&options.Port, "P", 22, "Port number")
	flagSet.BoolVar(&options.IsRemoteTo, "t", false, "Remote 'to' mode - not currently supported")
	flagSet.BoolVar(&options.IsRemoteFrom, "f", false, "Remote 'from' mode - not currently supported")
	flagSet.BoolVar(&options.IsQuiet, "q", false, "Quiet mode: disables the progress meter as well as warning and diagnostic messages")
	flagSet.BoolVar(&options.IsVerbose, "v", false, "Verbose mode - output differs from normal scp")
	flagSet.BoolVar(&options.IsCheckKnownHosts, "check-known-hosts", false, "Check known hosts - experimental!")
	flagSet.StringVar(&options.KeyFile, "key-file", "", "Use this keyfile to authenticate")
	err := flagSet.Parse(call[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Flag error:  %v\n\n", err.Error())
		flagSet.Usage()
		return err
	}
	if flagSet.ProcessHelpOrVersion() {
		return nil
	}

	if options.IsRemoteTo || options.IsRemoteFrom {
		return errors.New("This scp does NOT implement 'remote scp'. Yet.")
	}
	args := flagSet.Args()
	if len(args) != 2 {
		flagSet.Usage()
		return nil
	}

	srcFile, srcHost, srcUser, err := parseTarget(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Stderr, "Error parsing source")
		return err
	}
	dstFile, dstHost, dstUser, err := parseTarget(args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Stderr, "Error parsing destination")
		return err
	}
	if srcHost != "" && dstHost != "" {
		return errors.New("remote->remote NOT implemented (yet)!")
	} else if srcHost != "" {
		err = scpFromRemote(srcUser, srcHost, srcFile, dstFile, options)
		if err != nil {
			fmt.Fprintln(os.Stderr, os.Stderr, "Failed to run 'from-remote' scp: "+err.Error())
		}
		return err

	} else if dstHost != "" {
		err = scpToRemote(srcFile, dstUser, dstHost, dstFile, options)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run 'to-remote' scp: "+err.Error())
		}
		return err
	} else {
		srcReader, err := os.Open(srcFile)
		defer srcReader.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to open local source file ('local-local' scp): "+err.Error())
			return err
		}
		dstWriter, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY, 0777)
		defer dstWriter.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to open local destination file ('local-local' scp): "+err.Error())
			return err
		}
		n, err := io.Copy(dstWriter, srcReader)
		fmt.Printf("wrote %d bytes\n", n)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run 'local-local' copy: "+err.Error())
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

func connect(userName, host string, port int, idFile string, checkKnownHosts bool, verbose bool) (*ssh.Session, error) {
	if userName == "" { //check 
		u, err := user.Current()
		if err != nil {
			//never mind (probably cross-compiled. $USER usually does the trick)
			userName = os.Getenv("USER")
		} else {
			userName = u.Username
		}
	}
	auths := []ssh.ClientAuth{}
	if idFile != "" {
		//load and sign
		keyring := clientKeyring{}
		err := keyring.LoadFromPEMFile(idFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading key file (%v)\n", err)
		} else {
			auths = append ( auths, ssh.ClientAuthKeyring(&keyring) )
		}
	} else {
		sshAuthSock := os.Getenv("SSH_AUTH_SOCK")
		if sshAuthSock != "" {
			if agentClient, err := net.Dial("unix", sshAuthSock); err == nil {
				auths = append(auths, ssh.ClientAuthAgent(ssh.NewAgentClient(agentClient)))
			} else {
				fmt.Fprintln(os.Stderr, "Could not connect to ssh-agent", err)
			}
		} else {
			if verbose {
				fmt.Fprintln(os.Stderr, "Did not load ssh-agent because SSH_AUTH_SOCK not available.")
			}
		}
	}

	auths = append ( auths, ssh.ClientAuthPassword(passwordPrompt{userName, host, ""}) )
	clientConfig := &ssh.ClientConfig{
		User: userName,
		Auth: auths,
	}
	if checkKnownHosts {
		clientConfig.HostKeyChecker = loadKnownHosts(verbose)
	}
	target := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", target, clientConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to dial: "+err.Error())
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create session: "+err.Error())
	}
	return session, err

}
