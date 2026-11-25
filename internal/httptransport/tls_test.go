package httptransport

import (
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadOrCreateCert_GeneratesValidSelfSignedLeaf(t *testing.T) {
	dir := t.TempDir()

	cert, err := loadOrCreateCert(dir, time.Now, discardLogger())
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

	first, err := loadOrCreateCert(dir, time.Now, discardLogger())
	if err != nil {
		t.Fatalf("loadOrCreateCert: %v", err)
	}
	second, err := loadOrCreateCert(dir, time.Now, discardLogger())
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

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func readCertKey(t *testing.T, dir string) (certPEM, keyPEM []byte) {
	t.Helper()
	c, err := os.ReadFile(dir + "/" + certFileName)
	if err != nil {
		t.Fatal(err)
	}
	k, err := os.ReadFile(dir + "/" + keyFileName)
	if err != nil {
		t.Fatal(err)
	}
	return c, k
}

func TestLoadOrCreateCert_ValidCertReusedByteForByte(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	if _, err := loadOrCreateCert(dir, func() time.Time { return base }, discardLogger()); err != nil {
		t.Fatal(err)
	}
	c1, k1 := readCertKey(t, dir)

	later := base.Add(certValidity - certRenewWindow - 24*time.Hour)
	if _, err := loadOrCreateCert(dir, func() time.Time { return later }, discardLogger()); err != nil {
		t.Fatal(err)
	}
	c2, k2 := readCertKey(t, dir)
	if string(c1) != string(c2) || string(k1) != string(k2) {
		t.Error("a certificate outside the renewal window was rewritten")
	}
}

func TestLoadOrCreateCert_RegeneratesExpiredAndNearExpiry(t *testing.T) {
	for name, offset := range map[string]time.Duration{
		"expired":     certValidity + 24*time.Hour,
		"near expiry": certValidity - certRenewWindow + 24*time.Hour,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			base := time.Now()
			if _, err := loadOrCreateCert(dir, func() time.Time { return base }, discardLogger()); err != nil {
				t.Fatal(err)
			}
			c1, k1 := readCertKey(t, dir)

			later := base.Add(offset)
			cert, err := loadOrCreateCert(dir, func() time.Time { return later }, discardLogger())
			if err != nil {
				t.Fatal(err)
			}
			c2, k2 := readCertKey(t, dir)
			if string(c1) == string(c2) || string(k1) == string(k2) {
				t.Error("certificate was not regenerated")
			}
			leaf, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			if !leaf.NotAfter.After(later.Add(certRenewWindow)) {
				t.Errorf("regenerated NotAfter %v is still inside the renewal window", leaf.NotAfter)
			}
			for _, n := range []string{certFileName, keyFileName} {
				info, err := os.Stat(dir + "/" + n)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0o600 {
					t.Errorf("%s mode = %o, want 0600", n, info.Mode().Perm())
				}
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 2 {
				t.Errorf("state dir has %d entries, want 2 (temp file left behind?)", len(entries))
			}
		})
	}
}

func TestLoadOrCreateCert_CorruptOrMismatchedStillErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadOrCreateCert(dir, time.Now, discardLogger()); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if _, err := loadOrCreateCert(other, time.Now, discardLogger()); err != nil {
		t.Fatal(err)
	}
	_, otherKey := readCertKey(t, other)
	if err := os.WriteFile(dir+"/"+keyFileName, otherKey, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateCert(dir, time.Now, discardLogger()); err == nil {
		t.Error("mismatched cert/key must error, not be silently regenerated")
	}

	if err := os.WriteFile(dir+"/"+certFileName, []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateCert(dir, time.Now, discardLogger()); err == nil {
		t.Error("corrupt cert must error, not be silently regenerated")
	}
}

func TestLoadOrCreateCert_InterruptedRenewalRecovers(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	if _, err := loadOrCreateCert(dir, func() time.Time { return base }, discardLogger()); err != nil {
		t.Fatal(err)
	}
	later := base.Add(certValidity + 24*time.Hour)

	orig := writeFile
	t.Cleanup(func() { writeFile = orig })
	writeFile = func(path string, data []byte, perm os.FileMode) error {
		if strings.HasSuffix(path, certFileName) {
			return errors.New("injected cert write failure")
		}
		return orig(path, data, perm)
	}
	if _, err := loadOrCreateCert(dir, func() time.Time { return later }, discardLogger()); err == nil {
		t.Fatal("expected the injected cert write failure")
	}
	writeFile = orig

	cert, err := loadOrCreateCert(dir, func() time.Time { return later }, discardLogger())
	if err != nil {
		t.Fatalf("next start must recover from an interrupted renewal: %v", err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if !leaf.NotAfter.After(later.Add(certRenewWindow)) {
		t.Error("recovered certificate is still inside the renewal window")
	}
}
