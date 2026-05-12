package monobank

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Permissions.
const (
	// PermSt - statements(transactions) and client info of individual(фізичної особи).
	PermSt = "s"
	// PermPI - personal information(first and last names).
	PermPI = "p"
	// PermFOP - statements(transactions) and client info of private entrepreneur(ФОП).
	PermFOP = "f"
)

// Errors.
var (
	ErrDecodePrivateKey  = errors.New("failed to decode private key")
	ErrEncodePublicKey   = errors.New("failed to encode public key with sha1")
	ErrNoPrivateKey      = errors.New("failed to find private key block")
	ErrInvalidEC         = errors.New("invalid elliptic curve private key value")
	ErrInvalidPrivateKey = errors.New("invalid private key length")
)

type CorpAuthMaker struct {
	privateKey *ecdsa.PrivateKey
	KeyID      string // X-Key-Id - ID key of the service
}

type ecPrivateKey struct {
	Version       int
	PrivateKey    []byte
	NamedCurveOID asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	PublicKey     asn1.BitString        `asn1:"optional,explicit,tag:1"`
}

const (
	ecPrivateKeyBlockType = "EC PRIVATE KEY"
	ecPrivateKeyVersion   = 1
)

func NewCorpAuthMaker(secKey []byte) (*CorpAuthMaker, error) {
	privateKey, err := decodePrivateKey(secKey)
	if err != nil {
		return nil, ErrDecodePrivateKey
	}

	publicKey := privateKey.PublicKey
	data := elliptic.Marshal(publicKey, publicKey.X, publicKey.Y)
	hash := sha1.New()
	if _, err := hash.Write(data); err != nil {
		return nil, ErrEncodePublicKey
	}
	keyID := hex.EncodeToString(hash.Sum(nil))

	return &CorpAuthMaker{
		privateKey: privateKey,
		KeyID:      keyID,
	}, nil
}

func (c *CorpAuthMaker) New(requestID string) Authorizer {
	return CorpAuth{
		CorpAuthMaker: c,
		requestID:     requestID,
	}
}

func (c *CorpAuthMaker) NewPermissions(permissions ...string) Authorizer {
	return CorpAuth{
		CorpAuthMaker: c,
		permissions:   strings.Join(permissions, ""),
	}
}

type CorpAuth struct {
	*CorpAuthMaker
	requestID   string // Request ID(tokenRequestId)
	permissions string // Permissions
}

func (a CorpAuth) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}

	var actor string
	switch {
	case a.requestID != "":
		actor = a.requestID
		r.Header.Set("X-Request-Id", actor)
	case a.permissions != "":
		actor = a.permissions
		r.Header.Set("X-Permissions", actor)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	if r.URL == nil {
		return errors.New("missing URL in request")
	}
	sign, err := a.sign(timestamp, actor, r.URL.Path)
	if err != nil {
		return fmt.Errorf("calculate Sign: %w", err)
	}

	r.Header.Set("X-Key-Id", a.KeyID)
	r.Header.Set("X-Time", timestamp)
	r.Header.Set("X-Sign", sign)

	return nil
}

// sign - calculates Sign (X-time | X-Request-Id/X-Permissions | URL)
func (a CorpAuth) sign(timestamp, actor, urlPath string) (string, error) {
	return a.signString(timestamp + actor + urlPath)
}

func (a CorpAuth) signString(str string) (string, error) {
	hash := sha256.Sum256([]byte(str))

	r, s, err := ecdsa.Sign(rand.Reader, a.privateKey, hash[:])
	if err != nil {
		return "", err
	}

	asn1Data := []*big.Int{r, s}

	bb, err := asn1.Marshal(asn1Data)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(bb), nil
}

// decodePrivateKey extracts an ECDSA private key from PEM-encoded SEC1
// (`EC PRIVATE KEY`) data. Uses secp256k1's typed helpers from
// dcrd/dcrec/secp256k1/v4 instead of reimplementing x509.parseECPrivateKey.
func decodePrivateKey(b []byte) (*ecdsa.PrivateKey, error) {
	for {
		var block *pem.Block
		block, b = pem.Decode(b)
		if block == nil {
			return nil, ErrNoPrivateKey
		}
		if block.Type != ecPrivateKeyBlockType {
			continue
		}
		return parseECPrivateKey(block.Bytes)
	}
}

// parseECPrivateKey reads a SEC1 ASN.1 EC private-key blob and constructs
// an ecdsa.PrivateKey on secp256k1.
func parseECPrivateKey(b []byte) (*ecdsa.PrivateKey, error) {
	var privKey ecPrivateKey
	if _, err := asn1.Unmarshal(b, &privKey); err != nil {
		return nil, fmt.Errorf("failed to parse EC private key: %w", err)
	}
	if privKey.Version != ecPrivateKeyVersion {
		return nil, fmt.Errorf("unknown EC private key version %d", privKey.Version)
	}

	curve := secp256k1.S256()
	// SEC1 allows leading zeros; secp256k1 expects exactly 32 bytes. Strip
	// padding and validate that what's left is in the curve order.
	raw := privKey.PrivateKey
	expected := (curve.Params().N.BitLen() + 7) / 8
	for len(raw) > expected && raw[0] == 0 {
		raw = raw[1:]
	}
	if len(raw) > expected {
		return nil, ErrInvalidPrivateKey
	}

	if new(big.Int).SetBytes(raw).Cmp(curve.Params().N) >= 0 {
		return nil, ErrInvalidEC
	}

	padded := make([]byte, expected)
	copy(padded[expected-len(raw):], raw)

	priv := secp256k1.PrivKeyFromBytes(padded).ToECDSA()
	return priv, nil
}
