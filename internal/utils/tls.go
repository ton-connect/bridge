package utils

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/ton-connect/bridge/internal/models"
	"golang.org/x/crypto/nacl/box"
)

// GenerateSelfSignedCertificate generates a self-signed X.509 certificate and private key
func GenerateSelfSignedCertificate() ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	certTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"TON"},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(1000 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, &certTemplate, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	key, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key})

	return certPEM, keyPEM, nil
}

// EncryptRequestSourceWithWalletID encrypts the request source metadata using the wallet's Curve25519 public key
func EncryptRequestSourceWithWalletID(requestSource models.BridgeRequestSource, walletID string) (string, error) {
	data, err := json.Marshal(requestSource)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request source: %w", err)
	}

	publicKeyBytes, err := hex.DecodeString(walletID)
	if err != nil {
		return "", fmt.Errorf("failed to decode wallet ID: %w", err)
	}

	if len(publicKeyBytes) != 32 {
		return "", fmt.Errorf("invalid public key length: expected 32 bytes, got %d", len(publicKeyBytes))
	}

	// Convert to Curve25519 public key format
	var recipientPublicKey [32]byte
	copy(recipientPublicKey[:], publicKeyBytes)

	encrypted, err := box.SealAnonymous(nil, data, &recipientPublicKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encrypted), nil
}
