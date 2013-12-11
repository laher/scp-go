package scp

import (
	"testing"
	"net"
)
const (
	knownHost1 = "192.168.3.99"
	knownHost2 = "|1|NcRR/Ygr6aS/eCU01dPoQAP8F60=|8Cip+TT/Inx8zpljsGyypTSMy6A="
)
var (
	hostsMap = map[string][]byte{ knownHost1: []byte{ 1 },
		knownHost2: []byte{2} }
)

func TestKnownHash(t *testing.T) {
	err := checkHashedHost(knownHost2, knownHost1)
	if err != nil {
		t.Errorf("hash check failed %v", err)
	}
}

func XTestKnownHostsBasic(t *testing.T) {
	//
	khc := KnownHostsKeyChecker{hostsMap, nil, nil, true}
	host := "127.0.0.1"
	key := []byte{ 0, 0, 0, 7, 1 }
	remote, err := net.ResolveIPAddr("ip4", "192.168.3.99")
	if err != nil {
		t.Errorf("ip resolution failed %v", err)
	}
	alg := "blah"
	
	err = khc.Check(host, remote, alg, key)
	if err != nil {
		t.Errorf("check failed %v", err)
	}
}
