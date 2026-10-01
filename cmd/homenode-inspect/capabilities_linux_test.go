//go:build linux

package main

import (
	"strings"
	"testing"
)

func TestInspectionRequiresEveryCapabilitySetEmpty(t *testing.T) {
	status := "Name:\tfixture\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapBnd:\t0000000000000000\nCapAmb:\t0000000000000000\n"
	if !emptyInspectionCapabilities([]byte(status)) {
		t.Fatal("empty capability sets rejected")
	}
	for _, key := range []string{"CapInh:", "CapPrm:", "CapEff:", "CapBnd:", "CapAmb:"} {
		for _, changed := range []string{
			strings.Replace(status, key+"\t0000000000000000", key+"\t0000000000000001", 1),
			strings.Replace(status, key+"\t0000000000000000\n", "", 1),
			status + key + "\t0000000000000000\n",
			strings.Replace(status, key+"\t0000000000000000", key+"\tinvalid", 1),
		} {
			if emptyInspectionCapabilities([]byte(changed)) {
				t.Fatal("capability authority accepted", key)
			}
		}
	}
}
