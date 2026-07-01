package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// VAPIDKeys is the application server identity for Web Push. The public key is
// handed to browsers so they can create a subscription bound to this server; the
// private key signs the JWT on every push and is a secret.
type VAPIDKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

const vapidFileName = "vapid.json"

// LoadOrCreateVAPIDKeys returns the persisted VAPID keypair under dir, generating
// and saving a fresh one on first use. The keypair must be stable across daemon
// restarts: rotating it would invalidate every browser subscription already tied
// to the old public key. The file is written 0600 as it holds a private key.
func LoadOrCreateVAPIDKeys(dir string) (VAPIDKeys, error) {
	path := filepath.Join(dir, vapidFileName)
	if data, err := os.ReadFile(path); err == nil {
		var k VAPIDKeys
		if err := json.Unmarshal(data, &k); err == nil && k.Public != "" && k.Private != "" {
			return k, nil
		}
	}

	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return VAPIDKeys{}, fmt.Errorf("generate vapid keys: %w", err)
	}
	k := VAPIDKeys{Public: strings.TrimSpace(pub), Private: strings.TrimSpace(priv)}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return VAPIDKeys{}, fmt.Errorf("create push dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(k, "", "  ")
	if err != nil {
		return VAPIDKeys{}, fmt.Errorf("encode vapid keys: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return VAPIDKeys{}, fmt.Errorf("write vapid keys %s: %w", path, err)
	}
	return k, nil
}
