package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
	"github.com/Servora-Kit/plateau/security/jwt"
)

func TestKeygenGenerateIdempotentCorruptAndForceRotate(t *testing.T) {
	dir := t.TempDir()
	secretsDir := filepath.Join(dir, "secrets")
	signingPath := filepath.Join(secretsDir, "oidc-signing.pem")
	cryptoPath := filepath.Join(secretsDir, "oidc-crypto.key")

	var out bytes.Buffer
	if err := execute([]string{"-signing-key", signingPath, "-crypto-key", cryptoPath}, &out, io.Discard); err != nil {
		t.Fatalf("first execute failed: %v", err)
	}
	if !strings.Contains(out.String(), "OIDC signing key generated:") || !strings.Contains(out.String(), "OIDC crypto key generated:") {
		t.Fatalf("unexpected first execute output:\n%s", out.String())
	}

	signingBytes1, err := os.ReadFile(signingPath)
	if err != nil {
		t.Fatalf("read signing key: %v", err)
	}
	cryptoBytes1, err := os.ReadFile(cryptoPath)
	if err != nil {
		t.Fatalf("read crypto key: %v", err)
	}

	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(secretsDir)
		if err != nil {
			t.Fatalf("stat secrets dir: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0o700 {
			t.Errorf("secrets dir perm = %04o; want 0700", perm)
		}
		for _, path := range []string{signingPath, cryptoPath} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat %s: %v", path, err)
			}
			if perm := info.Mode().Perm(); perm != 0o600 {
				t.Errorf("%s perm = %04o; want 0600", path, perm)
			}
		}
	}

	signer, err := jwt.NewSigner(signingBytes1)
	if err != nil {
		t.Fatalf("jwt.NewSigner rejected generated signing key: %v", err)
	}
	decodedCrypto, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(cryptoBytes1)))
	if err != nil || len(decodedCrypto) != 32 {
		t.Fatalf("decoded crypto key len=%d err=%v; want 32 bytes base64url", len(decodedCrypto), err)
	}

	// Second execution without -force must skip and preserve identical bytes.
	out.Reset()
	if err := execute([]string{"-signing-key", signingPath, "-crypto-key", cryptoPath}, &out, io.Discard); err != nil {
		t.Fatalf("second execute failed: %v", err)
	}
	if !strings.Contains(out.String(), "OIDC signing key skip (valid):") || !strings.Contains(out.String(), "OIDC crypto key skip (valid):") {
		t.Fatalf("unexpected second execute output:\n%s", out.String())
	}
	signingBytes2, err := os.ReadFile(signingPath)
	if err != nil {
		t.Fatalf("read signing key after skip: %v", err)
	}
	cryptoBytes2, err := os.ReadFile(cryptoPath)
	if err != nil {
		t.Fatalf("read crypto key after skip: %v", err)
	}
	if !bytes.Equal(signingBytes1, signingBytes2) {
		t.Fatal("idempotent run modified signing key")
	}
	if !bytes.Equal(cryptoBytes1, cryptoBytes2) {
		t.Fatal("idempotent run modified crypto key")
	}

	// Corrupted signing key without -force must fail without overwriting corrupted content.
	corruptContent := []byte("corrupt-pem-content")
	if err := os.WriteFile(signingPath, corruptContent, 0o600); err != nil {
		t.Fatalf("write corrupt signing key: %v", err)
	}
	out.Reset()
	if err := execute([]string{"-signing-key", signingPath, "-crypto-key", cryptoPath}, &out, io.Discard); err == nil {
		t.Fatal("expected error for corrupted signing key without -force")
	}
	afterCorrupt, err := os.ReadFile(signingPath)
	if err != nil {
		t.Fatalf("read signing key after failed run: %v", err)
	}
	if !bytes.Equal(corruptContent, afterCorrupt) {
		t.Fatal("failed run overwrote corrupted signing key without -force")
	}

	// Restore signing key and verify corrupted crypto key without -force also fails without overwriting.
	if err := os.WriteFile(signingPath, signingBytes1, 0o600); err != nil {
		t.Fatalf("restore signing key: %v", err)
	}
	corruptCrypto := []byte("too-short")
	if err := os.WriteFile(cryptoPath, corruptCrypto, 0o600); err != nil {
		t.Fatalf("write corrupt crypto key: %v", err)
	}
	out.Reset()
	if err := execute([]string{"-signing-key", signingPath, "-crypto-key", cryptoPath}, &out, io.Discard); err == nil {
		t.Fatal("expected error for corrupted crypto key without -force")
	}
	afterCorruptCrypto, err := os.ReadFile(cryptoPath)
	if err != nil {
		t.Fatalf("read crypto key after failed run: %v", err)
	}
	if !bytes.Equal(corruptCrypto, afterCorruptCrypto) {
		t.Fatal("failed run overwrote corrupted crypto key without -force")
	}
	// Re-corrupt signing key so -force recovery covers both files.
	if err := os.WriteFile(signingPath, corruptContent, 0o600); err != nil {
		t.Fatalf("write corrupt signing key for force test: %v", err)
	}

	// -force rotates both keys and recovers from corrupted file.
	out.Reset()
	if err := execute([]string{"-signing-key", signingPath, "-crypto-key", cryptoPath, "-force"}, &out, io.Discard); err != nil {
		t.Fatalf("force rotate failed: %v", err)
	}
	signingBytes3, err := os.ReadFile(signingPath)
	if err != nil {
		t.Fatalf("read rotated signing key: %v", err)
	}
	cryptoBytes3, err := os.ReadFile(cryptoPath)
	if err != nil {
		t.Fatalf("read rotated crypto key: %v", err)
	}
	rotatedSigner, err := jwt.NewSigner(signingBytes3)
	if err != nil {
		t.Fatalf("jwt.NewSigner rejected rotated signing key: %v", err)
	}
	if rotatedSigner.KID() == signer.KID() {
		t.Fatal("force rotation produced identical signing key KID")
	}
	if bytes.Equal(cryptoBytes1, cryptoBytes3) {
		t.Fatal("force rotation produced identical crypto key")
	}

	if _, kid, _, err := oidc.LoadSigningKey(signingPath); err != nil || kid == "" {
		t.Fatalf("oidc.LoadSigningKey readback kid=%q err=%v", kid, err)
	}
	if _, kid, err := oidc.LoadCryptoKey(cryptoPath); err != nil || kid == "" {
		t.Fatalf("oidc.LoadCryptoKey readback kid=%q err=%v", kid, err)
	}
}

func TestKeygenResolveFromConfAndEnvOverride(t *testing.T) {
	confDir := t.TempDir()
	defaultSigning := filepath.Join(confDir, "secrets", "signing.pem")
	defaultCrypto := filepath.Join(confDir, "secrets", "crypto.key")
	envCrypto := filepath.Join(confDir, "override", "crypto-env.key")

	oidcYAML := fmt.Sprintf(`oidc:
  issuer: "http://iam.plateau.localhost:8080"
  signing_key_path: %q
  crypto_key_path: "${TEST_KEYGEN_CRYPTO_PATH:%s}"
  clients:
    - client_id: admin
      client_secret: "secret-123456"
      allowed_grant_types: [client_credentials]
      audiences: [iam]
`, defaultSigning, filepath.ToSlash(defaultCrypto))

	if err := os.WriteFile(filepath.Join(confDir, "oidc.yaml"), []byte(oidcYAML), 0o600); err != nil {
		t.Fatalf("write oidc.yaml: %v", err)
	}

	t.Setenv("TEST_KEYGEN_CRYPTO_PATH", envCrypto)

	var out bytes.Buffer
	if err := execute([]string{"-conf", confDir}, &out, io.Discard); err != nil {
		t.Fatalf("execute -conf failed: %v", err)
	}

	if _, err := os.Stat(defaultSigning); err != nil {
		t.Fatalf("expected signing key at %s: %v", defaultSigning, err)
	}
	if _, err := os.Stat(envCrypto); err != nil {
		t.Fatalf("expected env-overridden crypto key at %s: %v", envCrypto, err)
	}
	if _, err := os.Stat(defaultCrypto); !os.IsNotExist(err) {
		t.Fatalf("default crypto path %s should not have been created when overridden", defaultCrypto)
	}
}

func TestKeygenRejectsUnexpectedArgsAndPrintsHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := execute([]string{"force"}, &out, &errOut); err == nil {
		t.Fatal("expected error for unexpected positional argument")
	}

	out.Reset()
	errOut.Reset()
	err := execute([]string{"-h"}, &out, &errOut)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("execute(-h) error = %v; want flag.ErrHelp", err)
	}
	if !strings.Contains(errOut.String(), "-force") {
		t.Fatalf("expected usage output on stderr, got: %q", errOut.String())
	}
}
