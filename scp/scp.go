package scp

// thanks to this for inspiration ... https://gist.github.com/jedy/3357393

import (
	"bufio"
	"bytes"
	"code.google.com/p/go.crypto/ssh"
	"errors"
	"fmt"
	"github.com/howeyc/gopass"
	"github.com/laher/uggo"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	VERSION = "0.2.1"
)

type ScpOptions struct {
	Port         int
	IsRecursive  bool
	IsRemoteTo   bool
	IsRemoteFrom bool
	IsQuiet      bool
	IsVerbose bool
	IsCheckKnownHosts bool
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
	fmt.Fprintf(os.Stderr, "Warning: this scp is incomplete and not currently working with all ssh servers\n")
	options := ScpOptions{}
	flagSet := uggo.NewFlagSetDefault("scp", "[options] [[user@]host1:]file1 [[user@]host2:]file2", VERSION)
	flagSet.BoolVar(&options.IsRecursive, "r", false, "Recursive copy")
	flagSet.IntVar(&options.Port, "P", 22, "Port number")
	flagSet.BoolVar(&options.IsRemoteTo, "t", false, "Remote 'to' mode - not currently supported")
	flagSet.BoolVar(&options.IsRemoteFrom, "f", false, "Remote 'from' mode - not currently supported")
	flagSet.BoolVar(&options.IsQuiet, "q", false, "Quiet mode: disables the progress meter as well as warning and diagnostic messages")
	flagSet.BoolVar(&options.IsVerbose, "v", false, "Verbose mode - output differs from normal scp")
	flagSet.BoolVar(&options.IsCheckKnownHosts, "check-known-hosts", false, "Check known hosts - experimental!")
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
			fmt.Fprintln(os.Stderr, os.Stderr, "Failed to run 'from-remote' scp: " + err.Error())
		}
		return err

	} else if dstHost != "" {
		err = scpToRemote(srcFile, dstUser, dstHost, dstFile, options)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run 'to-remote' scp: " + err.Error())
		}
		return err
	} else {
		srcReader, err := os.Open(srcFile)
		defer srcReader.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to open local source file ('local-local' scp): " + err.Error())
			return err
		}
		dstWriter, err := os.OpenFile(dstFile, os.O_CREATE|os.O_WRONLY, 0777)
		defer dstWriter.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to open local destination file ('local-local' scp): " + err.Error())
			return err
		}
		n, err := io.Copy(dstWriter, srcReader)
		fmt.Printf("wrote %d bytes\n", n)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to run 'local-local' copy: " + err.Error())
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

type KnownHostsKeyChecker struct {
	KnownHosts map[string][]byte
}

func (khkc KnownHostsKeyChecker) Check(addr string, remote net.Addr, algorithm string, hostKey []byte) error {
	hostport := strings.SplitN(addr, ":", 2)
	host := hostport[0]
	existingKey, keyExists := khkc.KnownHosts[host]
	if keyExists {
		existingKey = append([]byte{0,0,0,7}, existingKey...)
		fmt.Printf("Key found for host %s.\n", host)
		fmt.Printf("hostKey len: %d\n", len(hostKey))
		fmt.Printf("exisKey len: %d\n", len(existingKey))
		for i, b := range hostKey {
			if b != existingKey[i] {
				fmt.Printf("byte %d, thishost: %v, existing: %v\n", i, b, existingKey[i])
		//		return errors.New("Keys do not match!")
			}
		}
		if bytes.Equal(hostKey, existingKey) {
			fmt.Printf("Keys match!\n")
			return nil
		} else {
			fmt.Printf("hostKey: '%s'\n", hostKey)
			fmt.Printf("exisKey: '%s'\n", existingKey)
			return errors.New("Keys do not match!")
		}
	} else {
		fmt.Printf("Key not found for host %s. Accept?\n", host)
		return errors.New("Key not found. 'add' not implemented yet in scp-go")
	}
}

func userHomeDir() string {
	usr, err := user.Current()
	if err != nil {
		fmt.Printf("Could not get home directory: %s\n", err)
		return os.Getenv("HOME")
	}
	fmt.Printf("user dir: %s\n", usr.HomeDir)
	return usr.HomeDir

}

func loadKnownHosts() map[string][]byte {
	knownHosts := map[string][]byte{}
	sshDir := filepath.Join(userHomeDir(), ".ssh")
	_, err := os.Stat(sshDir)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist\n", sshDir)
		err := os.Mkdir(sshDir, 0777)
		if err != nil {
			fmt.Printf("Could not create %s\n", sshDir)
		}
		return knownHosts
	}
	knownHostsFile := filepath.Join(sshDir, "known_hosts")
	_, err = os.Stat(knownHostsFile)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist\n", knownHostsFile)
		return knownHosts
	}
	file, err := os.Open(knownHostsFile)
	if err != nil {
		fmt.Printf("Could not create %s\n", knownHostsFile)
		return knownHosts
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 || strings.HasPrefix(line, "#") {
			//ignore
		} else {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				fmt.Printf("Known host %s, type: %s\n", parts[0], parts[1]) // Println will add back the final '\n'
				pk, comment, options, rest, ok := ssh.ParseAuthorizedKey([]byte(parts[1]))
				if ok {
					fmt.Printf("OK known host key for %s || comment: %s, options: %v, rest: %v\n", parts[0], comment, options, rest)
					knownHosts[parts[0]] = append([]byte("ssh-rsa"), pk.Marshal()...)
					//knownHosts[parts[0]] = 
				} else {
					fmt.Printf("Could not decode hostkey %s\n", parts[1])
				}
			} else {
				fmt.Printf("Unparseable host %s\n", line)
			}
		}
	}
	return knownHosts
}

func hostKeyChecker() KnownHostsKeyChecker {
	knownHosts := loadKnownHosts()
	return KnownHostsKeyChecker{knownHosts}
}

//note: shouldn't the password check come after the host key check?
//Not sure if this is possible with crypto.ssh
func connect(userName, host string, port int, checkKnownHosts bool) (*ssh.Session, error) {
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
	if checkKnownHosts {
		clientConfig.HostKeyChecker = hostKeyChecker()
	}
	target := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", target, clientConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to dial: " + err.Error())
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create session: " + err.Error())
	}
	return session, err

}
