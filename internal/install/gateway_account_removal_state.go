package install

import (
	"fmt"
	"strings"
)

// gatewayRemovalState reconciles each removal acknowledgement. A missing private
// group after userdel is permitted because Linux may remove it automatically.
// Reconstructing only removed rows lets the creation validator enforce the same
// ownership, numeric collision and isolation rules throughout teardown.
func gatewayRemovalState(s accountSnapshot, p GatewayAccountRemovalPlan) (int, error) {
	users, err := accountLines(s.passwd, 7)
	if err != nil {
		return 0, err
	}
	groups, err := accountLines(s.groups, 4)
	if err != nil {
		return 0, err
	}
	secrets, err := accountLines(s.shadow, 9)
	if err != nil {
		return 0, err
	}
	user, secret, private, proxy, controller := false, false, false, false, false
	for _, row := range users {
		if row[0] == "homenode-gateway" {
			user = true
		}
	}
	for _, row := range secrets {
		if row[0] == "homenode-gateway" {
			secret = true
		}
	}
	var normalized strings.Builder
	for _, row := range groups {
		switch row[0] {
		case "homenode-gateway":
			private = true
			if !user && row[3] != "" {
				return 0, ErrConflict
			}
		case "homenode-proxy":
			proxy = true
			members := strings.Split(row[3], ",")
			for _, member := range members {
				if member == "homenode" {
					controller = true
				}
			}
			if !user && row[3] != "" {
				return 0, ErrConflict
			}
			if user && !controller && row[3] != "homenode-gateway" {
				return 0, ErrConflict
			}
			if !controller {
				row[3] = "homenode,homenode-gateway"
			}
		}
		normalized.WriteString(strings.Join(row, ":") + "\n")
	}
	if user != secret || (user && (!private || !proxy)) || (!user && controller) {
		return 0, ErrConflict
	}
	phase := 0
	if !controller {
		phase = 1
	}
	if !user {
		phase = 2
	}
	if !proxy {
		phase = 3
	}
	if !proxy && !private {
		phase = 4
	}
	// Proxy deletion cannot precede user deletion.
	if !proxy && user {
		return 0, ErrConflict
	}
	if !user {
		s.passwd = append(append([]byte(nil), s.passwd...), []byte(fmt.Sprintf("homenode-gateway:x:%d:%d:HomeNode install %s:/nonexistent:/usr/sbin/nologin\n", p.Identity.UID, p.Identity.GID, p.OwnerID))...)
		s.shadow = append(append([]byte(nil), s.shadow...), []byte("homenode-gateway:!:1:0:99999:7:::\n")...)
	}
	if !private {
		normalized.WriteString(fmt.Sprintf("homenode-gateway:x:%d:\n", p.Identity.GID))
	}
	if !proxy {
		normalized.WriteString(fmt.Sprintf("homenode-proxy:x:%d:homenode,homenode-gateway\n", p.Identity.ProxyGID))
	}
	s.groups = []byte(normalized.String())
	matched, err := gatewayAccountStepMatches(s, GatewayAccountPlan{OwnerID: p.OwnerID, Identity: p.Identity}, 3)
	if err != nil {
		return 0, err
	}
	if !matched {
		return 0, ErrConflict
	}
	return phase, nil
}
