package scp

import (
	"bufio"
	"bytes"
	"code.google.com/p/go.crypto/ssh"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type KnownHostsKeyChecker struct {
	KnownHosts map[string][]byte
	verbose bool
}

func (khkc KnownHostsKeyChecker) Check(addr string, remote net.Addr, algorithm string, hostKey []byte) error {
	hostport := strings.SplitN(addr, ":", 2)
	host := hostport[0]
	existingKey, hostFound := khkc.KnownHosts[host]
	if !hostFound {
		//check by hash
		for k, v := range khkc.KnownHosts {
			if strings.HasPrefix(k, "|1|") {
				parts := strings.Split(k, "|")
				if len(parts) > 3 {
					salt := parts[2]
					knownHash := parts[3]
					//hash check
					//fmt.Printf("|%s|%s|\n", salt, host)
					saltDecoded, err := base64.StdEncoding.DecodeString(salt)
					if err != nil {
						return err
					}
					h := sha1.New()
					_, err = h.Write(saltDecoded)
					if err != nil {
						return err
					}
					//io.WriteString(h, salt)
					_, err = io.WriteString(h, host)
					if err != nil {
						return err
					}
					out := h.Sum(nil)
					hashed := base64.StdEncoding.EncodeToString(out)
					if khkc.verbose {
						fmt.Printf("Generated %s (with salt %s + host %s. Comparing to %s)\n", hashed, salt, host, knownHash)
					}
					if hashed == knownHash {
						existingKey = v
						hostFound = true
					}
				} else {
					fmt.Printf("Invalid hashed host line\n")
				}
			}
		}
	}
	if hostFound {
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


func loadKnownHosts(verbose bool) map[string][]byte {
	knownHosts := map[string][]byte{}
	sshDir := filepath.Join(userHomeDir(verbose), ".ssh")
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
				if verbose {
					fmt.Printf("Known host %s, type: %s\n", parts[0], parts[1]) // Println will add back the final '\n'
				}
				pk, comment, options, rest, ok := ssh.ParseAuthorizedKey([]byte(parts[1]))
				if ok {
					if verbose {
						fmt.Printf("OK known host key for %s || comment: %s, options: %v, rest: %v\n", parts[0], comment, options, rest)
					}
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

func hostKeyChecker(verbose bool) KnownHostsKeyChecker {
	knownHosts := loadKnownHosts(verbose)
	return KnownHostsKeyChecker{knownHosts, verbose}
}

func userHomeDir(verbose bool) string {
	usr, err := user.Current()
	if err != nil {
		fmt.Printf("Could not get home directory: %s\n", err)
		return os.Getenv("HOME")
	}
	if verbose {
		fmt.Printf("user dir: %s\n", usr.HomeDir)
	}
	return usr.HomeDir

}
