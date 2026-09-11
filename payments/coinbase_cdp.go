package payments

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"discord-bot/config"
)

// cdpHost is the Coinbase App API host used for account/address operations.
const cdpHost = "api.coinbase.com"

// cdpClient talks to the Coinbase App API (v2) using CDP JWT authentication.
// This is the "normal account" address flow: it generates receive addresses
// rather than hosted Commerce checkout pages.
type cdpClient struct {
	cfg     *config.CoinbasePaymentConfig
	keyName string
	priv    ed25519.PrivateKey
	http    *http.Client

	// accountIDs caches asset code -> account id lookups.
	accountIDs map[string]string
}

func newCDPClient(cfg *config.CoinbasePaymentConfig) (*cdpClient, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.CDPPrivateKey))
	if err != nil {
		// tolerate URL-safe base64 as well
		raw, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(cfg.CDPPrivateKey))
		if err != nil {
			return nil, fmt.Errorf("coinbase cdp: decode private key: %w", err)
		}
	}

	var priv ed25519.PrivateKey
	switch len(raw) {
	case ed25519.SeedSize: // 32
		priv = ed25519.NewKeyFromSeed(raw)
	case ed25519.PrivateKeySize: // 64 (seed || public)
		priv = ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
	default:
		return nil, fmt.Errorf("coinbase cdp: unexpected Ed25519 key length %d (want 32 or 64)", len(raw))
	}

	return &cdpClient{
		cfg:        cfg,
		keyName:    strings.TrimSpace(cfg.CDPKeyName),
		priv:       priv,
		http:       &http.Client{Timeout: 30 * time.Second},
		accountIDs: make(map[string]string),
	}, nil
}

// generateJWT builds a short-lived (2 min) EdDSA JWT bound to a single request.
// uri is "METHOD host/path", e.g. "GET api.coinbase.com/v2/accounts".
func (c *cdpClient) generateJWT(method, path string) (string, error) {
	nonceBytes := make([]byte, 8)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(nonceBytes) // 16 hex chars

	header := map[string]interface{}{
		"alg":   "EdDSA",
		"typ":   "JWT",
		"kid":   c.keyName,
		"nonce": nonce,
	}
	now := time.Now().Unix()
	claims := map[string]interface{}{
		"sub": c.keyName,
		"iss": "cdp",
		"aud": []string{"cdp_service"},
		"nbf": now,
		"exp": now + 120,
		"uri": fmt.Sprintf("%s %s%s", method, cdpHost, path),
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	sig := ed25519.Sign(c.priv, []byte(signingInput))
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// doAuthed performs an authenticated Coinbase App API request. path must start
// with "/" (e.g. "/v2/accounts"). Returns body, status, error.
func (c *cdpClient) doAuthed(method, path string, body interface{}) ([]byte, int, error) {
	// The uri claim must not include the query string.
	uriPath := path
	if i := strings.IndexByte(uriPath, '?'); i != -1 {
		uriPath = uriPath[:i]
	}
	jwt, err := c.generateJWT(method, uriPath)
	if err != nil {
		return nil, 0, err
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, "https://"+cdpHost+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("CB-VERSION", "2024-01-01")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

// GetSpotPrice returns the price of one unit of asset in fiat, using Coinbase's
// public (unauthenticated) spot price endpoint.
func (c *cdpClient) GetSpotPrice(asset, fiat string) (float64, error) {
	url := fmt.Sprintf("https://%s/v2/prices/%s-%s/spot", cdpHost, strings.ToUpper(asset), strings.ToUpper(fiat))
	resp, err := c.http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("coinbase spot price %s-%s (HTTP %d): %s", asset, fiat, resp.StatusCode, string(b))
	}
	var res struct {
		Data struct {
			Amount string `json:"amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, err
	}
	price, err := strconv.ParseFloat(res.Data.Amount, 64)
	if err != nil || price <= 0 {
		return 0, fmt.Errorf("coinbase spot price %s-%s: invalid amount %q", asset, fiat, res.Data.Amount)
	}
	return price, nil
}

// GetAccountID finds the wallet account id for a given asset code (e.g. "BTC").
func (c *cdpClient) GetAccountID(asset string) (string, error) {
	asset = strings.ToUpper(asset)
	if id, ok := c.accountIDs[asset]; ok {
		return id, nil
	}

	path := "/v2/accounts?limit=100"
	for path != "" {
		b, status, err := c.doAuthed("GET", path, nil)
		if err != nil {
			return "", err
		}
		if status != 200 {
			return "", fmt.Errorf("coinbase list accounts (HTTP %d): %s", status, string(b))
		}
		var res struct {
			Data []struct {
				ID       string `json:"id"`
				Currency struct {
					Code string `json:"code"`
				} `json:"currency"`
			} `json:"data"`
			Pagination struct {
				NextURI string `json:"next_uri"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(b, &res); err != nil {
			return "", err
		}
		for _, a := range res.Data {
			if strings.ToUpper(a.Currency.Code) == asset {
				c.accountIDs[asset] = a.ID
				return a.ID, nil
			}
		}
		path = res.Pagination.NextURI
	}
	return "", fmt.Errorf("coinbase: no %s account found on this Coinbase profile", asset)
}

// CreateAddress generates a fresh receive address on the given account. network
// is optional (e.g. "base" for USDC).
func (c *cdpClient) CreateAddress(accountID, name, network string) (address, addressID string, err error) {
	payload := map[string]interface{}{}
	if name != "" {
		payload["name"] = name
	}
	if network != "" {
		payload["network"] = network
	}
	b, status, err := c.doAuthed("POST", "/v2/accounts/"+accountID+"/addresses", payload)
	if err != nil {
		return "", "", err
	}
	if status != 200 && status != 201 {
		return "", "", fmt.Errorf("coinbase create address (HTTP %d): %s", status, string(b))
	}
	var res struct {
		Data struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return "", "", err
	}
	if res.Data.Address == "" {
		return "", "", fmt.Errorf("coinbase create address: empty address in response: %s", string(b))
	}
	return res.Data.Address, res.Data.ID, nil
}

// AddressReceived returns the total positive amount received at an address, in
// the address's asset units.
func (c *cdpClient) AddressReceived(accountID, addressID string) (float64, error) {
	b, status, err := c.doAuthed("GET", "/v2/accounts/"+accountID+"/addresses/"+addressID+"/transactions", nil)
	if err != nil {
		return 0, err
	}
	if status != 200 {
		return 0, fmt.Errorf("coinbase address transactions (HTTP %d): %s", status, string(b))
	}
	var res struct {
		Data []struct {
			Status string `json:"status"`
			Amount struct {
				Amount string `json:"amount"`
			} `json:"amount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return 0, err
	}
	var total float64
	for _, t := range res.Data {
		if t.Status != "completed" && t.Status != "confirmed" {
			continue
		}
		v, perr := strconv.ParseFloat(t.Amount.Amount, 64)
		if perr == nil && v > 0 {
			total += v
		}
	}
	return total, nil
}

// formatCryptoAmount formats a crypto amount with sensible per-asset precision.
func formatCryptoAmount(asset string, amount float64) string {
	decimals := 8
	switch strings.ToUpper(asset) {
	case "USDC", "USDT", "DAI", "EURC":
		decimals = 2
	}
	s := strconv.FormatFloat(amount, 'f', decimals, 64)
	// trim trailing zeros but keep at least 2 decimal places
	if dot := strings.IndexByte(s, '.'); dot != -1 {
		s = strings.TrimRight(s, "0")
		frac := len(s) - strings.IndexByte(s, '.') - 1
		if frac == 0 {
			s += "00"
		} else if frac == 1 {
			s += "0"
		}
	}
	return s
}
