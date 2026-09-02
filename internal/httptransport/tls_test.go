package httptransport

import (
	"crypto/x509"
	"net"
	"os"
	"testing"
)

func TestLoadOrCreateCert_GeneratesValidSelfSignedLeaf(t *testing.T) {
	dir := t.TempDir()

	cert, err := loadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("loadOrCreateCert: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Fatal("expected at least one DER certificate")
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse generated certificate: %v", err)
	}

	if leaf.IsCA {
		t.Error("generated leaf certificate has IsCA=true; a serving cert used directly must not claim CA capability")
	}
	if leaf.KeyUsage&x509.KeyUsageCertSign != 0 {
		t.Error("generated leaf certificate has KeyUsageCertSign set")
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("generated leaf certificate is missing KeyUsageDigitalSignature")
	}

	wantIP := net.ParseIP("127.0.0.1")
	found := false
	for _, ip := range leaf.IPAddresses {
		if ip.Equal(wantIP) {
			found = true
		}
	}
	if !found {
		t.Error("generated certificate does not cover 127.0.0.1")
	}

	for _, path := range []string{certFileName, keyFileName} {
		info, err := os.Stat(dir + "/" + path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s permissions = %o, want 0600", path, perm)
		}
	}
}

func TestLoadOrCreateCert_ReloadsExisting(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("loadOrCreateCert: %v", err)
	}
	second, err := loadOrCreateCert(dir)
	if err != nil {
		t.Fatalf("loadOrCreateCert (reload): %v", err)
	}

	if len(first.Certificate) == 0 || len(second.Certificate) == 0 {
		t.Fatal("expected certificate bytes on both loads")
	}
	if string(first.Certificate[0]) != string(second.Certificate[0]) {
		t.Error("reloading produced a different certificate — it was not persisted")
	}
}
