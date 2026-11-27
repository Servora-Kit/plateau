package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	oidcconfpb "github.com/Servora-Kit/plateau/api/gen/go/plateau/iam/oidc/conf/v1"
	"github.com/Servora-Kit/plateau/app/iam/service/internal/oidc"
	"github.com/Servora-Kit/servora/core/bootstrap"
	"github.com/Servora-Kit/servora/core/bootstrap/config"
)

const toolName = "iam.keygen"

func main() {
	if err := execute(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintf(os.Stderr, "keygen: %v\n", err)
		os.Exit(1)
	}
}

func execute(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet(toolName, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		confPath       string
		signingKeyPath string
		cryptoKeyPath  string
		force          bool
	)
	fs.StringVar(&confPath, "conf", "./configs/local/", "config path, eg: -conf ./configs/local/")
	fs.StringVar(&signingKeyPath, "signing-key", "", "explicit OIDC RSA signing key PEM path (overrides -conf)")
	fs.StringVar(&cryptoKeyPath, "crypto-key", "", "explicit OIDC 32-byte crypto key path (overrides -conf)")
	fs.BoolVar(&force, "force", false, "force rotation/overwrite of existing OIDC key files")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}

	signingKeyPath = strings.TrimSpace(signingKeyPath)
	cryptoKeyPath = strings.TrimSpace(cryptoKeyPath)

	if signingKeyPath == "" || cryptoKeyPath == "" {
		resolvedSigning, resolvedCrypto, err := resolvePathsFromConfig(confPath)
		if err != nil {
			return err
		}
		if signingKeyPath == "" {
			signingKeyPath = resolvedSigning
		}
		if cryptoKeyPath == "" {
			cryptoKeyPath = resolvedCrypto
		}
	}

	signingCreated, keyID, err := ensureSigningKeyFile(signingKeyPath, force)
	if err != nil {
		return err
	}
	if signingCreated {
		_, _ = fmt.Fprintf(stdout, "OIDC signing key generated: %s (kid=%s)\n", signingKeyPath, keyID)
	} else {
		_, _ = fmt.Fprintf(stdout, "OIDC signing key skip (valid): %s (kid=%s)\n", signingKeyPath, keyID)
	}

	cryptoCreated, err := ensureCryptoKeyFile(cryptoKeyPath, force)
	if err != nil {
		return err
	}
	if cryptoCreated {
		_, _ = fmt.Fprintf(stdout, "OIDC crypto key generated: %s\n", cryptoKeyPath)
	} else {
		_, _ = fmt.Fprintf(stdout, "OIDC crypto key skip (valid): %s\n", cryptoKeyPath)
	}

	return nil
}

func resolvePathsFromConfig(confPath string) (string, string, error) {
	_, cfg, err := config.LoadBootstrap(confPath, toolName, false)
	if err != nil {
		return "", "", fmt.Errorf("load config %q: %w", confPath, err)
	}
	defer func() { _ = cfg.Close() }()

	oidcCfg := &oidcconfpb.OIDC{}
	if err := bootstrap.Scan(&bootstrap.Runtime{Config: cfg}, oidcCfg); err != nil {
		return "", "", fmt.Errorf("scan OIDC config from %q: %w", confPath, err)
	}
	signingKeyPath := strings.TrimSpace(oidcCfg.GetSigningKeyPath())
	cryptoKeyPath := strings.TrimSpace(oidcCfg.GetCryptoKeyPath())
	if signingKeyPath == "" {
		return "", "", fmt.Errorf("signing_key_path is empty in %q", confPath)
	}
	if cryptoKeyPath == "" {
		return "", "", fmt.Errorf("crypto_key_path is empty in %q", confPath)
	}
	return signingKeyPath, cryptoKeyPath, nil
}

func ensureSigningKeyFile(path string, force bool) (bool, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, "", fmt.Errorf("OIDC signing key path is empty")
	}
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, "", fmt.Errorf("OIDC signing key path %q is a directory", path)
		}
		if !force {
			_, keyID, _, loadErr := oidc.LoadSigningKey(path)
			if loadErr != nil {
				return false, "", fmt.Errorf("existing OIDC signing key %q is invalid (use -force to overwrite): %w", path, loadErr)
			}
			return false, keyID, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, "", fmt.Errorf("stat OIDC signing key: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return false, "", fmt.Errorf("generate OIDC RSA signing key: %w", err)
	}
	pkcs8DER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return false, "", fmt.Errorf("marshal OIDC PKCS#8 signing key: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8DER,
	})
	keyID, err := writeKeyFileAtomic(path, pemBytes, func(candidate string) (string, error) {
		_, kid, _, verifyErr := oidc.LoadSigningKey(candidate)
		return kid, verifyErr
	})
	if err != nil {
		return false, "", err
	}
	return true, keyID, nil
}

func ensureCryptoKeyFile(path string, force bool) (bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return false, fmt.Errorf("OIDC crypto key path is empty")
	}
	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("OIDC crypto key path %q is a directory", path)
		}
		if !force {
			if _, _, loadErr := oidc.LoadCryptoKey(path); loadErr != nil {
				return false, fmt.Errorf("existing OIDC crypto key %q is invalid (use -force to overwrite): %w", path, loadErr)
			}
			return false, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat OIDC crypto key: %w", err)
	}

	var raw [32]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return false, fmt.Errorf("generate OIDC crypto key: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw[:]) + "\n"
	if _, err := writeKeyFileAtomic(path, []byte(encoded), func(candidate string) (string, error) {
		_, _, verifyErr := oidc.LoadCryptoKey(candidate)
		return "", verifyErr
	}); err != nil {
		return false, err
	}
	return true, nil
}

func writeKeyFileAtomic(path string, content []byte, validate func(string) (string, error)) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create key directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".oidc-key-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary key file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("chmod temporary key file: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write temporary key file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync temporary key file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temporary key file: %w", err)
	}
	meta, err := validate(tmpPath)
	if err != nil {
		return "", fmt.Errorf("validate generated key file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return "", fmt.Errorf("install key file: %w", err)
	}
	tmpPath = ""
	return meta, nil
}
