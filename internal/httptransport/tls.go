package httptransport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	certFileName = "cert.pem"
	keyFileName  = "key.pem"
	certValidity = 397 * 24 * time.Hour // just under the CA/Browser Forum's 398-day max
)

// certRenewWindow is how far ahead of NotAfter a persisted certificate is
// regenerated, so a long-running deployment never serves one that lapses
// between restarts.
const certRenewWindow = 30 * 24 * time.Hour

// writeFile is a seam so tests can fail a write mid-renewal.
var writeFile = writeFileAtomic

// loadOrCreateCert resolves the TLS certificate used to serve the listener:
// an existing cert/key pair in the state dir, or a freshly generated
// self-signed one written there. A persisted certificate that has expired or
// expires within certRenewWindow is regenerated, because clients reject an
// expired one and nothing else would ever replace it. The cert and key are
// never stored under the vault path.
func loadOrCreateCert(dir string, now func() time.Time, logger *slog.Logger) (tls.Certificate, error) {
	certPath := filepath.Join(dir, certFileName)
	keyPath := filepath.Join(dir, keyFileName)

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	switch {
	case err == nil:
		leaf, perr := x509.ParseCertificate(cert.Certificate[0])
		if perr != nil {
			return tls.Certificate{}, fmt.Errorf("parse existing cert: %w", perr)
		}
		if leaf.NotAfter.After(now().Add(certRenewWindow)) {
			return cert, nil
		}
		logger.Info("persisted TLS certificate expired or near expiry; regenerating",
			"not_after", leaf.NotAfter.UTC().Format(time.RFC3339))
	case !os.IsNotExist(err):
		return tls.Certificate{}, fmt.Errorf("load existing cert/key: %w", err)
	}

	certPEM, keyPEM, err := generateSelfSigned(now())
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate self-signed cert: %w", err)
	}
	// Drop the old cert first: a crash between the two writes then leaves the
	// cert missing (regenerated next start) rather than a new key beside the
	// old cert, which LoadX509KeyPair rejects as a mismatch.
	if err := os.Remove(certPath); err != nil && !os.IsNotExist(err) {
		return tls.Certificate{}, fmt.Errorf("remove old cert file %s: %w", certPath, err)
	}
	if err := writeFile(keyPath, keyPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write key file %s: %w", keyPath, err)
	}
	if err := writeFile(certPath, certPEM, 0o600); err != nil {
		return tls.Certificate{}, fmt.Errorf("write cert file %s: %w", certPath, err)
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}

// writeFileAtomic writes data to a temp file in path's directory and renames
// it over path, so a crash never leaves a truncated certificate or key.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func generateSelfSigned(now time.Time) (certPEM, keyPEM []byte, err error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "obsidian-mcp (self-signed)"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal key: %w", err)
	}

	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}

// loadClientCAPool reads a PEM file of trusted client CA certificates for
// mTLS.
func loadClientCAPool(path string) (*x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read client CA file %s: %w", path, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil, fmt.Errorf("no valid certificates found in client CA file %s", path)
	}
	return pool, nil
}
