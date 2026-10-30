package supervisor

import (
	"errors"
	"strings"
	"testing"
)

func TestDomainPIDParser(t *testing.T) {
	for _, test := range []struct {
		data string
		pid  int
	}{{"1", 1}, {"12345\n", 12345}, {"2147483647\n", 2147483647}} {
		pid, err := parseDomainPID([]byte(test.data))
		if err != nil || pid != test.pid {
			t.Fatal(pid, err)
		}
	}
	for _, data := range []string{"", "0", "-1", "+1", "01", "1\n\n", " 1", "1 ", "1\r\n", "1\x00", "2147483648", strings.Repeat("1", 33)} {
		if _, err := parseDomainPID([]byte(data)); !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified PID accepted", err)
		}
	}
}
