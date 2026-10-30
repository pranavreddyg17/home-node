package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestQEMUFDFlags(t *testing.T) {
	for _, test := range []struct {
		data  string
		flags uint32
	}{{"pos:\t0\nflags:\t0100002\nmnt_id:\t3\n", 0100002}, {"flags: 0\n", 0}, {"flags:\t2140002\n", 02140002}} {
		flags, err := parseQEMUFDFlags([]byte(test.data))
		if err != nil || flags != test.flags {
			t.Fatal(flags, err)
		}
	}
	for _, data := range []string{"", "pos: 0\n", "flags:\n", "flags: -1\n", "flags: +1\n", "flags: 8\n", "flags: 1 2\n", "flags: 0x10\n", "flags: 40000000000\n", "flags: 1\nflags: 1\n", "flags: 1\r\n", "flags: 1\x00\n", string([]byte{255}), strings.Repeat("x", 8193)} {
		if _, err := parseQEMUFDFlags([]byte(data)); !errors.Is(err, ErrPolicy) {
			t.Fatal("malformed fdinfo accepted", err)
		}
	}
}
