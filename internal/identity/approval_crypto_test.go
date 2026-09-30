package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pranavreddyg17/home-node/internal/state"
)

// This constructs a real ES256 assertion, not a mocked verifier result.
// It proves server crypto/origin/UV checks, not physical authenticator UX.
func TestApprovalSignedAssertionAndVerificationBoundaries(t *testing.T) {
	for _, mode := range []string{"valid", "wrong origin", "missing UV", "bad signature"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s := testService(t)
			actor, _ := testDevice(t, s, AllCapabilities)
			if _, err := s.Store.DB.Exec("UPDATE identity SET claimed=1"); err != nil {
				t.Fatal(err)
			}
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			public, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: key.X.FillBytes(make([]byte, 32)), -3: key.Y.FillBytes(make([]byte, 32))})
			if err != nil {
				t.Fatal(err)
			}
			credential := webauthn.Credential{ID: []byte(state.Random()), PublicKey: public}
			data, _ := json.Marshal(credential)
			if _, err := s.Store.DB.Exec("INSERT INTO credentials(id,device_id,data) VALUES(?,?,?)", credential.ID, actor.Device.ID, string(data)); err != nil {
				t.Fatal(err)
			}
			_, token, binding, err := s.BeginApproval(ctx, actor, "device.pair", []string{"phone"}, []byte("request body"), 1)
			if err != nil {
				t.Fatal(err)
			}
			var payload string
			if err := s.Store.DB.QueryRow("SELECT payload FROM challenges WHERE token_hash=?", state.Hash(token)).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			var ceremony challenge
			if err := json.Unmarshal([]byte(payload), &ceremony); err != nil {
				t.Fatal(err)
			}
			origin := "http://localhost:8787"
			if mode == "wrong origin" {
				origin = "https://attacker.example"
			}
			clientData, _ := json.Marshal(map[string]any{"type": "webauthn.get", "challenge": ceremony.Session.Challenge, "origin": origin, "crossOrigin": false})
			rp := sha256.Sum256([]byte("localhost"))
			authData := append([]byte{}, rp[:]...)
			flags := byte(5) // user presence plus user verification
			if mode == "missing UV" {
				flags = 1
			}
			authData = append(authData, flags, 0, 0, 0, 0)
			binary.BigEndian.PutUint32(authData[33:], 1)
			clientHash := sha256.Sum256(clientData)
			signed := append(append([]byte{}, authData...), clientHash[:]...)
			digest := sha256.Sum256(signed)
			signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
			if err != nil {
				t.Fatal(err)
			}
			if mode == "bad signature" {
				signature[len(signature)-1] ^= 1
			}
			enc := base64.RawURLEncoding.EncodeToString
			assertion, _ := json.Marshal(map[string]any{"id": enc(credential.ID), "rawId": enc(credential.ID), "type": "public-key", "response": map[string]string{"clientDataJSON": enc(clientData), "authenticatorData": enc(authData), "signature": enc(signature)}, "clientExtensionResults": map[string]any{}})
			request := httptest.NewRequest("POST", origin, strings.NewReader(string(assertion)))
			request.Header.Set("Content-Type", "application/json")
			grant, err := s.FinishApproval(ctx, actor, token, 1, request)
			var count int
			if e := s.Store.DB.QueryRow("SELECT count(*) FROM challenges WHERE kind='approval-grant'").Scan(&count); e != nil {
				t.Fatal(e)
			}
			if mode != "valid" {
				if err == nil || grant != "" || count != 0 {
					t.Fatal("invalid assertion issued grant", err, count)
				}
				return
			}
			if err != nil || grant == "" || count != 1 {
				t.Fatal("signed assertion failed", err, count)
			}
			var stored string
			if err := s.Store.DB.QueryRow("SELECT payload FROM challenges WHERE token_hash=?", state.Hash(grant)).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			var issued challenge
			if err := json.Unmarshal([]byte(stored), &issued); err != nil || issued.Binding == nil || issued.Binding.digest() != binding.digest() {
				t.Fatal("issued authority changed", err)
			}
			if err := s.Store.DB.QueryRow("SELECT data FROM credentials WHERE id=?", credential.ID).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(stored), &credential); err != nil || credential.Authenticator.SignCount != 1 {
				t.Fatal("counter not persisted", err)
			}
		})
	}
}
