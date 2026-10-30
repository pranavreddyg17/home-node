package supervisor

import (
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
)

func TestQEMUSocketMetadata(t *testing.T) {
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108, ChannelPath: "/run/homenode/instance/adapter.sock"}
	monitor, err := qemuMonitorPath(d, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"candidate-adapter-listener", "candidate-monitor-listener", "candidate-monitor-connected"} {
		t.Run(role, func(t *testing.T) {
			socket := qemuSocketIdentity{qemuRegularIdentity: qemuRegularIdentity{Device: 10, Inode: 100, Links: 1, Mode: 0140000 | 0777}, Family: 1, Type: 1, Listening: 1, Local: monitor}
			if role == "candidate-adapter-listener" {
				socket.Local = d.ChannelPath
			}
			if role == "candidate-monitor-connected" {
				socket.Listening = 0
				socket.Peer = "@"
				socket.PeerPID = 100
				socket.UID = d.GuestUID
				socket.GID = d.GuestGID
			}
			if got, err := qualifyQEMUSocket(d, 3, socket, 2); err != nil || got != role {
				t.Fatal(got, err)
			}
			for _, mutation := range []string{"owner", "group", "mode", "links", "inode", "family", "type", "protocol", "listening", "path", "peer", "peer-pid", "peer-owner", "peer-group", "access", "runtime", "id", "uid", "gid", "channel"} {
				t.Run(mutation, func(t *testing.T) {
					domain, s, runtimeID, access := d, socket, uint32(3), uint32(2)
					switch mutation {
					case "owner":
						s.UID++
					case "group":
						s.GID++
					case "mode":
						s.Mode ^= 1
					case "links":
						s.Links = 2
					case "inode":
						s.Inode = 0
					case "family":
						s.Family = 2
					case "type":
						s.Type = 2
					case "protocol":
						s.Protocol = 1
					case "listening":
						s.Listening = 2
					case "path":
						s.Local = "/run/foreign.sock"
					case "peer":
						s.Peer = "/run/foreign.sock"
					case "peer-pid":
						if s.Listening == 1 {
							s.PeerPID = 100
						} else {
							s.PeerPID = 0
						}
					case "peer-owner":
						s.PeerUID++
					case "peer-group":
						s.PeerGID++
					case "access":
						access = 0
					case "runtime":
						runtimeID = 0
					case "id":
						domain.ID = "../foreign"
					case "uid":
						domain.GuestUID = 0
					case "gid":
						domain.GuestGID = 0
					case "channel":
						domain.ChannelPath = "relative.sock"
					}
					if got, err := qualifyQEMUSocket(domain, runtimeID, s, access); got != "" || !errors.Is(err, ErrPolicy) {
						t.Fatal("unqualified socket admitted", got, err)
					}
				})
			}
		})
	}
}
