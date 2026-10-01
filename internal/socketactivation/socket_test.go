package socketactivation

import "testing"

func TestActivationEnvironmentRequiresExactSingleNamedDescriptor(t *testing.T) {
	for _, scenario := range []string{"valid", "foreign-pid", "extra-fds", "unnamed", "foreign-name", "pidfd", "noncanonical-count", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			values := map[string]string{"LISTEN_PID": "123", "LISTEN_FDS": "1", "LISTEN_FDNAMES": "backup-apps"}
			switch scenario {
			case "foreign-pid":
				values["LISTEN_PID"] = "124"
			case "extra-fds":
				values["LISTEN_FDS"] = "2"
			case "unnamed":
				values["LISTEN_FDNAMES"] = ""
			case "foreign-name":
				values["LISTEN_FDNAMES"] = "runtime"
			case "pidfd":
				values["LISTEN_PIDFDID"] = "42"
			case "noncanonical-count":
				values["LISTEN_FDS"] = "01"
			case "missing":
				delete(values, "LISTEN_PID")
			}
			if validEnvironment(func(key string) string { return values[key] }, 123, "backup-apps") != (scenario == "valid") {
				t.Fatal("unexpected activation admission")
			}
		})
	}
}
