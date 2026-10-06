package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGuestUIDNameServicesRequireExplicitLocalResolution(t *testing.T) {
	valid := "passwd: files\ngroup: files\nshadow: files\nsubid: files\nhosts: files dns\n"
	if err := validateGuestUIDNameServices(context.Background(), []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(valid, "passwd: files", "passwd: files systemd", 1),
		strings.Replace(valid, "group: files", "group: files sss", 1),
		strings.Replace(valid, "subid: files", "subid: sss", 1),
		strings.Replace(valid, "shadow: files", "shadow: compat", 1),
		strings.Replace(valid, "subid: files\n", "", 1),
		valid + "passwd: files\n",
		strings.Replace(valid, "passwd: files", "passwd: files [SUCCESS=return]", 1),
		valid + "malformed\n", valid + "\x00", valid + "\r",
	} {
		if err := validateGuestUIDNameServices(context.Background(), []byte(data)); !errors.Is(err, ErrPolicy) {
			t.Fatal("unsupported name service accepted", data, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateGuestUIDNameServices(ctx, []byte(valid)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
