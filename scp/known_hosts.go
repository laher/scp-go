package scp

import (
	"bufio"
	"bytes"
	"code.google.com/p/go.crypto/ssh"
	"crypto/sha1"
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type KnownHostsKeyChecker struct {
	KnownHosts map[string][]byte
	RevokedHosts map[string][]byte
	CAHosts map[string][]byte
	verbose bool
}

func checkHashedHost(knownHost string, host string) error {
	if strings.HasPrefix(knownHost, "|1|") {
		parts := strings.Split(knownHost, "|")
		if len(parts) > 3 {
			salt := parts[2]
			knownHash := parts[3]
			//hash check
			//fmt.Printf("|%s|%s|\n", salt, host)
			saltDecoded, err := base64.StdEncoding.DecodeString(salt)
			//salt decoded
			//fmt.Printf("%d|% x|\n", len(saltDecoded), saltDecoded)
			if err != nil {
				return err
			}
			h := hmac.New(sha1.New, saltDecoded)
			//h := sha1.New()
			//_, err = h.Write(saltDecoded)
			//if err != nil {
			//	return err
			//}
			//io.WriteString(h, salt)
			//_, err = io.WriteString(h, host)
			_, err = h.Write([]byte(host))
			//fmt.Printf("%d|% x|\n", len(host), []byte(host))
			if err != nil {
				return err
			}
			out := h.Sum(nil)
			//fmt.Printf("%d|% x|\n", len(out), out)
			hashed := base64.StdEncoding.EncodeToString(out)
			//if khkc.verbose {
			//}
			if hashed == knownHash {
				//fmt.Printf("Matched %s=%s (with salt %s + host %s)\n", hashed, knownHash, salt, host)
				return nil
			} else {
				//fmt.Printf("Not Matched %s=%s (with salt %s + host %s)\n", hashed, knownHash, salt, host)
				//ignore line
			}
		} else {
			fmt.Printf("Invalid hashed host line\n")
		}
	} else {
		fmt.Printf("host line not hashed\n")
	}
	return errors.New("Not matched")
}
func parseWireKey(bs []byte, verbose bool) ssh.PublicKey {
	pk, rest, ok := ssh.ParsePublicKey(bs)
	if verbose {
		fmt.Printf("rest: %v, ok: %v\n", rest, ok)
	}
	return pk
}

func readHostFileKey(bs []byte, verbose bool) ssh.PublicKey {
	pk, comment, options, rest, ok := ssh.ParseAuthorizedKey(bs)
	if verbose {
		fmt.Printf("comment: %s, options: %v, rest: %v, ok: %v\n", comment, options, rest, ok)
	}
	return pk
}


func (khkc KnownHostsKeyChecker) matchHostWithHashSupport(host string) ([]byte, error) {
	existingKey, hostFound := khkc.KnownHosts[host]
	if !hostFound {
		//check by hash
		for k, v := range khkc.KnownHosts {
			err := checkHashedHost(k, host)
			if err != nil {
				//not matching
				//fmt.Printf("checkHashedHost failed")
			} else {
				//, v, hostKey)
				return v, nil
			}
		}
	} else {
		return existingKey, nil
	}
	return nil, errors.New("Not found")
}

func (khkc KnownHostsKeyChecker) Check(addr string, remote net.Addr, algorithm string, hostKey []byte) error {
	remoteAddr := remote.String()
	hostport := strings.SplitN(remoteAddr, ":", 2)
	host := hostport[0]
	existingKey, err := khkc.matchHostWithHashSupport(host)
	if err != nil {
		fmt.Printf("Key not found for host %s. Accept?\n", host)
		return errors.New("Key not found. 'add key' not implemented yet in scp-go")
	}

	existingPublicKey := readHostFileKey(existingKey, khkc.verbose)
/*
	splitAt := strings.Index(string(existingKey), " ")
	existingKeyAlg := string(hostKey[:splitAt])
	existingKeyVal := hostKey[splitAt+1:]
	encodedExistingKey := base64.StdEncoding.EncodeToString(existingKeyVal)
	encodedHostKey := base64.StdEncoding.EncodeToString(hostKey)
*/
	hostPublicKey := parseWireKey(hostKey, khkc.verbose)
	existingPKWireFormat := existingPublicKey.Marshal()
	hostPKWireFormat := hostPublicKey.Marshal()
	if bytes.Equal(hostPKWireFormat, existingPKWireFormat) {
	//if existingPublicKey.Marshal() == hostPublicKey {
		fmt.Printf("OK keys match")
		return nil
	} else {
		return errors.New("Key found but NOT matched!")
	}
/*
	hostKeyVal := hostKey[4:]
		//existingKey = append([]byte{0,0,0,7}, existingKey...)
		fmt.Printf("Key found for host %s.\n", host)
		fmt.Printf("h: %s, e: %s\n", algorithm, existingKeyAlg)
		fmt.Printf("hostKey len: %d\n", len(hostKeyVal))
		fmt.Printf("exisKey len: %d\n", len(existingKeyVal))
		for i, b := range hostKeyVal {
			if len(existingKeyVal) > i {
				if b != existingKeyVal[i] {
//					fmt.Printf("byte %d, thishost: %v, existing: %v\n", i, b, existingKey[i])
			//		return errors.New("Keys do not match!")
				}
			} else {
//				fmt.Printf("byte %d, thishost: %v, existing: nil\n", i, b)
			}
		}

		if algorithm != existingKeyAlg {
			return errors.New("Key types do NOT match!\n")

		}
		if bytes.Equal(hostKeyVal, existingKeyVal) {
			fmt.Printf("Keys match!\n")
			return nil
		} else {
			fmt.Printf("hostKey: '%s'\n", encodedHostKey)
			fmt.Printf("exisKey: '%s'\n", encodedExistingKey)
			return errors.New("Keys do not match!")
		}
	*/
	
}


func loadKnownHosts(verbose bool) KnownHostsKeyChecker {
	knownHosts := map[string][]byte{}
	revokedHosts := map[string][]byte{}
	caHosts := map[string][]byte{}
	khkc := KnownHostsKeyChecker{knownHosts, revokedHosts, caHosts, verbose}
	sshDir := filepath.Join(userHomeDir(verbose), ".ssh")
	_, err := os.Stat(sshDir)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist\n", sshDir)
		err := os.Mkdir(sshDir, 0777)
		if err != nil {
			fmt.Printf("Could not create %s\n", sshDir)
		}
		return khkc
	}
	knownHostsFile := filepath.Join(sshDir, "known_hosts")
	_, err = os.Stat(knownHostsFile)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist\n", knownHostsFile)
		return khkc
	}
	file, err := os.Open(knownHostsFile)
	if err != nil {
		fmt.Printf("Could not create %s\n", knownHostsFile)
		return khkc
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
				//check for revoked / ca / ...
				isRevoked := false
				isCa := false
				if parts[0] == "@revoked" {
					isRevoked = true
					parts = strings.SplitN(parts[1], " ", 2)
				}
				if parts[0] == "@cert-authority" {
					isCa = true
					parts = strings.SplitN(parts[1], " ", 2)
				}

				if verbose {
					fmt.Printf("Known host %s, type: %s\n", parts[0], parts[1]) // Println will add back the final '\n'
				}
				pk, comment, options, rest, ok := ssh.ParseAuthorizedKey([]byte(parts[1]))
				if ok {
					if verbose {
						fmt.Printf("OK known host key for %s |%s| comment: %s, options: %v, rest: %v\n", parts[0], base64.StdEncoding.EncodeToString(pk.Marshal()), comment, options, rest)
					}
					if verbose {
						fmt.Printf("Setting %s = %s\n", parts[0], parts[1])
					}
					//knownHosts[parts[0]] = append([]byte("ssh-rsa"), pk.Marshal()...)
					if isRevoked {
						revokedHosts[parts[0]] = []byte(parts[1])
					} else if isCa {
						caHosts[parts[0]] = []byte(parts[1])
					} else {
						knownHosts[parts[0]] = []byte(parts[1])
					}
					//knownHosts[parts[0]] = 
				} else {
					fmt.Printf("Could not decode hostkey %s\n", parts[1])
				}
			} else {
				fmt.Printf("Unparseable host %s\n", line)
			}
		}
	}
	return khkc
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
