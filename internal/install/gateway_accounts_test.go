package install

import (
	"strings"
	"testing"
)

func TestGatewayAccountIsolation(t *testing.T) {
	p, g, s := accountFixture()
	p += "homenode-gateway:x:803:803::/nonexistent:/usr/sbin/nologin\n"
	g += "homenode-gateway:x:803:\nhomenode-proxy:x:804:homenode,homenode-gateway\n"
	s += "homenode-gateway:!:1:0:99999:7:::\n"
	identity, err := ValidateGatewayAccount([]byte(p), []byte(g), []byte(s))
	if err != nil || identity != (GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}) {
		t.Fatal(identity, err)
	}
	if _, err := ValidateLocalAccounts([]byte(p), []byte(g), []byte(s)); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range [][3]string{
		{p, g + "backup:x:805:homenode-gateway\n", s},
		{p, strings.Replace(g, "802:homenode,homenode-transfer", "802:homenode,homenode-transfer,homenode-gateway", 1), s},
		{p, strings.Replace(g, "804:homenode,homenode-gateway", "804:homenode-transfer,homenode-gateway", 1), s},
		{p, g + "alias:x:804:\n", s},
		{p + "other:x:805:804::/nonexistent:/usr/sbin/nologin\n", g, s},
		{p + "other:x:803:805::/nonexistent:/usr/sbin/nologin\n", g, s},
		{p, g, strings.Replace(s, "homenode-gateway:!:", "homenode-gateway:hash:", 1)},
		{p, strings.Replace(g, "homenode-proxy:x:804:homenode,homenode-gateway\n", "", 1), s},
	} {
		if _, err := ValidateLocalAccounts([]byte(fixture[0]), []byte(fixture[1]), []byte(fixture[2])); err == nil {
			t.Fatal("gateway authority contamination accepted")
		}
	}
}
