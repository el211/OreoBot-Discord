package payments

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"discord-bot/config"
)

// TestCDPJWT verifies the generated JWT has the correct structure and a valid
// Ed25519 signature that verifies against the public key derived from the seed.
func TestCDPJWT(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// CDP secrets are the 64-byte (seed||public) key, base64-encoded.
	secret := base64.StdEncoding.EncodeToString(priv)
	keyName := "organizations/org-123/apiKeys/key-456"

	c, err := newCDPClient(&config.CoinbasePaymentConfig{
		CDPKeyName:    keyName,
		CDPPrivateKey: secret,
	})
	if err != nil {
		t.Fatalf("newCDPClient: %v", err)
	}

	tok, err := c.generateJWT("GET", "/v2/accounts")
	if err != nil {
		t.Fatalf("generateJWT: %v", err)
	}

	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	// header checks
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var hdr map[string]interface{}
	if err := json.Unmarshal(hb, &hdr); err != nil {
		t.Fatalf("header decode: %v", err)
	}
	if hdr["alg"] != "EdDSA" || hdr["typ"] != "JWT" || hdr["kid"] != keyName {
		t.Fatalf("bad header: %v", hdr)
	}
	if n, ok := hdr["nonce"].(string); !ok || len(n) != 16 {
		t.Fatalf("bad nonce: %v", hdr["nonce"])
	}

	// claim checks
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]interface{}
	if err := json.Unmarshal(cb, &claims); err != nil {
		t.Fatalf("claims decode: %v", err)
	}
	if claims["iss"] != "cdp" || claims["sub"] != keyName {
		t.Fatalf("bad claims: %v", claims)
	}
	if claims["uri"] != "GET api.coinbase.com/v2/accounts" {
		t.Fatalf("bad uri claim: %v", claims["uri"])
	}
	aud, ok := claims["aud"].([]interface{})
	if !ok || len(aud) != 1 || aud[0] != "cdp_service" {
		t.Fatalf("bad aud: %v", claims["aud"])
	}

	// signature must verify against the public key
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("sig decode: %v", err)
	}
	signingInput := parts[0] + "." + parts[1]
	if !ed25519.Verify(pub, []byte(signingInput), sig) {
		t.Fatal("signature failed to verify")
	}
}

func TestFormatCryptoAmount(t *testing.T) {
	cases := []struct {
		asset  string
		amount float64
		want   string
	}{
		{"BTC", 0.00042, "0.00042"},
		{"ETH", 0.5, "0.50"},
		{"USDC", 49.999, "50.00"},
		{"USDC", 12.5, "12.50"},
	}
	for _, tc := range cases {
		if got := formatCryptoAmount(tc.asset, tc.amount); got != tc.want {
			t.Errorf("formatCryptoAmount(%s, %v) = %q, want %q", tc.asset, tc.amount, got, tc.want)
		}
	}
}
