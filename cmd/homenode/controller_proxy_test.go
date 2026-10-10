package main

import "testing"

func TestControllerProxyConfiguration(t *testing.T) {
	if err := validateControllerProxy("linux", "/run/homenode-control/control.sock", false, 100, 100, 101, 102, "", ""); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, platform, socket string
		dev                    bool
		uid, gid               int
		gateway, bridge        uint32
		cert, key              string
	}{
		{"non-Linux", "darwin", "/run/control.sock", false, 100, 100, 101, 102, "", ""},
		{"development", "linux", "/run/control.sock", true, 100, 100, 101, 102, "", ""},
		{"root", "linux", "/run/control.sock", false, 0, 100, 101, 102, "", ""},
		{"shared UID", "linux", "/run/control.sock", false, 100, 100, 100, 102, "", ""},
		{"primary bridge", "linux", "/run/control.sock", false, 100, 100, 101, 100, "", ""},
		{"relative socket", "linux", "control.sock", false, 100, 100, 101, 102, "", ""},
		{"unclean socket", "linux", "/run/../control.sock", false, 100, 100, 101, 102, "", ""},
		{"TLS key authority", "linux", "/run/control.sock", false, 100, 100, 101, 102, "", "/key"},
		{"missing gateway", "linux", "/run/control.sock", false, 100, 100, 0, 102, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if validateControllerProxy(c.platform, c.socket, c.dev, c.uid, c.gid, c.gateway, c.bridge, c.cert, c.key) == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}
