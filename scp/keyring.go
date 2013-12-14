package scp

import (
	"bufio"
//	"bytes"
	"code.google.com/p/go.crypto/ssh"
//	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/rsa"
	"github.com/howeyc/gopass"
	"crypto/x509"
//	"encoding/base64"
	"encoding/pem"
//	"encoding/hex"
	"errors"
	"fmt"
//	"math/big"
	"io/ioutil"
	"io"
	"path/filepath"
	"os"
	"os/user"
)

// keyring implements the ClientKeyring interface
type clientKeyring struct {
	//should be interface type supporting PKCS#1, RSA, DSA and ECDSA 
        //keys []*rsa.PrivateKey
        signers []ssh.Signer
}

func (k *clientKeyring) Key(i int) (ssh.PublicKey, error) {
	//fmt.Println("Getting key number", i)
        if i < 0 || i >= len(k.signers) {
		//no more keys but no error. Signifies 'try next authenticator'
                return nil, nil
        }
        return k.signers[i].PublicKey(), nil
}

func (k *clientKeyring) Sign(i int, rand io.Reader, data []byte) (sig []byte, err error) {
	//fmt.Println("Signing with key number", i)
	return k.signers[i].Sign(rand, data)
}

func (k *clientKeyring) LoadRsa(key *rsa.PrivateKey) error {
	return k.load(key)
}

func (k *clientKeyring) LoadEcdsa(key *ecdsa.PrivateKey) error {
	return k.load(key)
}

func (k *clientKeyring) load(key interface{}) error {
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return err
	}
	//fmt.Printf("adding signer from key %+v\n with pub key: %+v\n", key, signer.PublicKey())
	k.signers = append(k.signers, signer)
	//fmt.Println("total ", len(k.signers), " signers")
	return nil
}

func userDir() string {
	u, err := user.Current()
	if err != nil {
		//probably cross-compiled. Use env
		return os.Getenv("HOME")
	}
	return u.HomeDir
}

func (k *clientKeyring) LoadIdFiles(files []string) []error {
	errs := []error{}
	for _, file := range files {
		
		err := k.LoadFromPEMFile(file)
		errs = append(errs, err)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading file '%s': \n\t%v\n", file, err)
		}
	}
	return errs
}

//todo: check openssh (ssh-add) source. should there be a glob on the dir?
func (k *clientKeyring) LoadDefaultIdFiles() []error {
	files := []string{
			filepath.Join(userDir(), ".ssh", "id_ecdsa"),
			filepath.Join(userDir(), ".ssh", "id_rsa"),
		//	can't get dsa working for now
		//	filepath.Join(userDir(), ".ssh", "id_dsa")
		}
	return k.LoadIdFiles(files)
}

func readLines(path string) ([]string, error) {
  file, err := os.Open(path)
  if err != nil {
    return nil, err
  }
  defer file.Close()

  var lines []string
  scanner := bufio.NewScanner(file)
  for scanner.Scan() {
    lines = append(lines, scanner.Text())
  }
  return lines, scanner.Err()
}

func (k *clientKeyring) LoadFromPEMFile(file string) error {
        filebuf, err := ioutil.ReadFile(file)
        if err != nil {
                return err
        }
        block, _ := pem.Decode(filebuf)
        if block == nil {
                return errors.New("ssh: no key found")
        }
	if x509.IsEncryptedPEMBlock(block) {
		fmt.Printf("Password for key '%s':", file)
		password := gopass.GetPasswd()
		decrypted, err := x509.DecryptPEMBlock(block, password)
		if err != nil {
			return err
		}
		//println("decrypted block of type ", block.Type)
		block.Bytes = decrypted
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		r, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		return k.LoadRsa(r)
	case "EC PRIVATE KEY":
		d, err := x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return err
		}
		return k.load(d)
/*
	case "DSA PRIVATE KEY":
		lines, err := readLines(file)
		important := lines[1:len(lines)-1]
		filebuf := ""
		for _, line := range important {
			filebuf += line
		}

		fmt.Printf("key data b64: %s\n", filebuf)
		dbytes, err := base64.StdEncoding.DecodeString(filebuf)
		if err != nil {
			return err
		}
		buf := bytes.NewBuffer(dbytes)

		var priv dsa.PrivateKey
		l := len(dbytes)
		fmt.Printf("dsa key length: %d\n", l)

		//if l != 448 {
		//	return errors.New("private key type '"+ block.Type + "' should be 404 bytes, but was not")
		//}
		//b := []byte{0,0,0,0}
		b := buf.Next(4)
		fmt.Printf("magic: %s\n", hex.EncodeToString(b))
		if err != nil {
			return err
		}
		fmt.Printf("All bits: %s\n", hex.EncodeToString(dbytes))
		fmt.Printf("All bits: %s\n", hex.EncodeToString(block.Bytes))
		priv.P = new(big.Int).SetBytes(block.Bytes[0:128])
		priv.Q = new(big.Int).SetBytes(block.Bytes[128:148])
		priv.G = new(big.Int).SetBytes(block.Bytes[148:286])
		//what about the other 160 bits?
		fmt.Printf("Missing bits: %s\n", hex.EncodeToString(block.Bytes[286:306]))
		priv.Y = new(big.Int).SetBytes(block.Bytes[306:424])
		priv.X = new(big.Int).SetBytes(block.Bytes[424:448])
		fmt.Printf("Missing bits: %s\n", hex.EncodeToString(block.Bytes[424:448]))
		
		return k.load(&priv)
*/
	default:
		return errors.New("Unsupported private key type '"+ block.Type + "'")
	}
}

