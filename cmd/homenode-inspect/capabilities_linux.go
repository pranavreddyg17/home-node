//go:build linux

package main

import (
	"strconv"
	"strings"
)

func emptyInspectionCapabilities(status []byte) bool {
	required := map[string]bool{"CapInh:": false, "CapPrm:": false, "CapEff:": false, "CapBnd:": false, "CapAmb:": false}
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		seen, ok := required[fields[0]]
		if !ok {
			continue
		}
		if seen || len(fields) != 2 || len(fields[1]) != 16 {
			return false
		}
		value, err := strconv.ParseUint(fields[1], 16, 64)
		if err != nil || value != 0 {
			return false
		}
		required[fields[0]] = true
	}
	for _, seen := range required {
		if !seen {
			return false
		}
	}
	return true
}
