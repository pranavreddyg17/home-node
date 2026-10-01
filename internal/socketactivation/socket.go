package socketactivation

import (
	"errors"
	"strconv"
)

var ErrListener = errors.New("required private socket activation invalid")

func validEnvironment(lookup func(string) string, pid int, name string) bool {
	return pid > 0 && name != "" && lookup("LISTEN_PID") == strconv.Itoa(pid) && lookup("LISTEN_FDS") == "1" && lookup("LISTEN_FDNAMES") == name && lookup("LISTEN_PIDFDID") == ""
}
