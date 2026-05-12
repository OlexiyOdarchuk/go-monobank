package monobank

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

// Webhook errors.
var (
	// ErrBadSignature is returned by VerifyWebhookSignature when the
	// signature does not match. Use errors.Is to detect it.
	ErrBadSignature = errors.New("webhook signature is invalid")
	// ErrBadSignatureEncoding is returned when X-Sign is not valid base64.
	ErrBadSignatureEncoding = errors.New("X-Sign is not valid base64")
	// ErrMissingPubKey is returned when verification is attempted with a nil key.
	ErrMissingPubKey = errors.New("missing public key")
	// ErrUnknownWebHookType is returned by ParseWebHook when the payload's
	// top-level "type" is not a recognised WebHookType* constant.
	ErrUnknownWebHookType = errors.New("unknown webhook type")
)

// VerifyWebhookSignature returns nil iff xSign is a valid ECDSA signature of
// body produced by the bank's serverPubKey.
//
//	sk, _ := client.ServerKey(ctx)
//	if err := monobank.VerifyWebhookSignature(sk.PubKey, body, r.Header.Get("X-Sign")); err != nil {
//	    // reject
//	}
//
// Mono currently encodes the signature as a raw 64-byte r||s pair (base64);
// ASN.1 DER is accepted as a fallback so future encoding changes don't break
// callers.
func VerifyWebhookSignature(pub *ecdsa.PublicKey, body []byte, xSign string) error {
	if pub == nil {
		return ErrMissingPubKey
	}
	sig, err := base64.StdEncoding.DecodeString(xSign)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadSignatureEncoding, err)
	}
	digest := sha256.Sum256(body)

	if len(sig) == 64 {
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if ecdsa.Verify(pub, digest[:], r, s) {
			return nil
		}
	}

	var asn1Sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &asn1Sig); err != nil {
		return ErrBadSignature
	}
	if asn1Sig.R == nil || asn1Sig.S == nil {
		return ErrBadSignature
	}
	if !ecdsa.Verify(pub, digest[:], asn1Sig.R, asn1Sig.S) {
		return ErrBadSignature
	}
	return nil
}

// Verify is a convenience wrapper around [VerifyWebhookSignature] using this
// key as the verifier.
func (k *ServerKey) Verify(body []byte, xSign string) error {
	if k == nil {
		return ErrMissingPubKey
	}
	return VerifyWebhookSignature(k.PubKey, body, xSign)
}

// ParseWebHook decodes a raw webhook body into a [WebHookResponse]. If the
// payload's "type" is not a known WebHookType* constant, the response is
// still returned but wrapped in [ErrUnknownWebHookType] so callers can opt
// out of processing unfamiliar events.
func ParseWebHook(body []byte) (*WebHookResponse, error) {
	var v WebHookResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, fmt.Errorf("decode webhook: %w", err)
	}
	switch v.Type {
	case WebHookTypeStatementItem:
		return &v, nil
	default:
		return &v, fmt.Errorf("%w: %q", ErrUnknownWebHookType, v.Type)
	}
}
