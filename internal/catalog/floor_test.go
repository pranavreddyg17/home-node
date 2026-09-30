package catalog

import "testing"

func TestVersionFloorCanonicalPositive(t *testing.T) {
	for _, input := range []string{"1", "4\n", "9223372036854775807"} {
		if n, err := VersionFloor([]byte(input)); err != nil || n < 1 {
			t.Fatal(input, n, err)
		}
	}
	for _, input := range []string{"", "0", "-1", "+1", "01", " 1", "1 ", "1\n\n", "1\r\n", "9223372036854775808", "1\x00", "1.0"} {
		if _, err := VersionFloor([]byte(input)); err == nil {
			t.Fatal("invalid floor admitted", input)
		}
	}
}
