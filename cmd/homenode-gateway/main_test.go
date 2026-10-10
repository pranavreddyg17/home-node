package main

import "testing"

func TestGatewayIdentitySeparation(t *testing.T) {
	for _, test := range []struct {
		platform           string
		uid, gid           int
		controller, bridge uint32
		groups             []int
		valid              bool
	}{
		{"linux", 200, 201, 202, 203, []int{201, 203}, true},
		{"darwin", 200, 201, 202, 203, []int{201, 203}, false},
		{"linux", 0, 201, 202, 203, []int{201, 203}, false},
		{"linux", 200, 0, 202, 203, []int{203}, false},
		{"linux", 200, 201, 200, 203, []int{201, 203}, false},
		{"linux", 200, 201, 202, 201, []int{201}, false},
		{"linux", 200, 201, 202, 203, []int{201, 203, 204}, false},
		{"linux", 200, 201, 0, 203, []int{201, 203}, false},
	} {
		if err := validateIdentity(test.platform, test.uid, test.gid, test.controller, test.bridge, test.groups); (err == nil) != test.valid {
			t.Fatal("identity separation mismatch", test, err)
		}
	}
}
