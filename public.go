package monobank

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

type PublicAPI interface {
	// Currency https://api.monobank.ua/docs/#/definitions/CurrencyInfo
	Currency(context.Context) (Currencies, error)

	// ServerKey fetches the bank's current public key, its identifier and
	// server time. Use it together with VerifyWebhookSignature.
	//
	// https://api.monobank.ua/bank/sync
	ServerKey(context.Context) (*ServerKey, error)
}

func (c Client) Currency(ctx context.Context) (Currencies, error) {
	const urlPath = "/bank/currency"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	var v Currencies
	err = c.do(req, &v, http.StatusOK)

	return v, err
}

// ErrInvalidPubKey is returned by [Client.ServerKey] when /bank/sync returns
// a serverPubKey that isn't a 65-byte uncompressed secp256k1 point.
var ErrInvalidPubKey = errors.New(
	"invalid serverPubKey: expected 65-byte uncompressed secp256k1 point")

// ServerKey fetches the bank's public key used to sign webhooks.
//
// The /bank/sync endpoint is public — no token is required. Cache the result
// and refresh whenever an incoming X-Key-Id stops matching ServerKey.ID; that
// is mono's signal that it rotated the key.
func (c Client) ServerKey(ctx context.Context) (*ServerKey, error) {
	const urlPath = "/bank/sync"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlPath, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	var raw struct {
		ServerKeyID    string `json:"serverKeyId"`
		ServerPubKey   string `json:"serverPubKey"`
		ServerTimeMsec int64  `json:"serverTimeMsec"`
	}
	if err := c.do(req, &raw, http.StatusOK); err != nil {
		return nil, err
	}

	pubBytes, err := base64.StdEncoding.DecodeString(raw.ServerPubKey)
	if err != nil {
		return nil, fmt.Errorf("decode serverPubKey: %w", err)
	}
	if len(pubBytes) != 65 || pubBytes[0] != 0x04 {
		return nil, ErrInvalidPubKey
	}

	return &ServerKey{
		ID: raw.ServerKeyID,
		PubKey: &ecdsa.PublicKey{
			Curve: secp256k1.S256(),
			X:     new(big.Int).SetBytes(pubBytes[1:33]),
			Y:     new(big.Int).SetBytes(pubBytes[33:]),
		},
		ServerTime: time.UnixMilli(raw.ServerTimeMsec),
	}, nil
}
