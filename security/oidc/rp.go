package oidc

import (
	"context"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	oidcv1 "github.com/Servora-Kit/plateau/api/gen/go/plateau/security/oidc/v1"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	httphelper "github.com/zitadel/oidc/v3/pkg/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"
)

const (
	handshakeCookieMaxAge = 300
	minCookieSecretBytes  = 32
	hkdfHashInfo          = "plateau/security/oidc/cookie-hash-v1"
	hkdfEncryptInfo       = "plateau/security/oidc/cookie-encrypt-v1"
)

type nonceContextKey struct{}

func withNonce(ctx context.Context, nonce string) context.Context {
	return context.WithValue(ctx, nonceContextKey{}, nonce)
}

func nonceFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	val, _ := ctx.Value(nonceContextKey{}).(string)
	return val
}

// VerifiedIdentity holds the verified OIDC claims established after a successful
// Authorization Code + PKCE S256 + nonce exchange.
type VerifiedIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
	AuthTime      time.Time
}

type handshakeState struct {
	State        string `json:"s"`
	Nonce        string `json:"n"`
	CodeVerifier string `json:"v"`
	ReturnTo     string `json:"r"`
}

// RelyingParty manages browser OAuth 2.1 / OIDC Authorization Code + PKCE S256
// login handshakes using short-lived HKDF-encrypted cookies and lazy discovery.
type RelyingParty struct {
	issuer       string
	clientID     string
	clientSecret string
	redirectURI  string
	scopes       []string
	cookieName   string
	cookies      *httphelper.CookieHandler
	httpClient   *http.Client
	timeout      time.Duration

	rp atomic.Pointer[rp.RelyingParty]
	sf singleflight.Group
}

// NewRelyingParty validates local OIDC RP configuration and derives independent
// HMAC and AES-256 cookie keys via HKDF without making network calls at startup.
func NewRelyingParty(config *oidcv1.OIDCClient, opts ...Option) (*RelyingParty, error) {
	if config == nil {
		return nil, fmt.Errorf("oidc: config is nil")
	}
	cfg := proto.Clone(config).(*oidcv1.OIDCClient)
	if err := cfg.Apply(); err != nil {
		return nil, fmt.Errorf("oidc: config: %w", err)
	}

	issuerURL, err := parseEndpointURL(cfg.GetIssuer())
	if err != nil {
		return nil, fmt.Errorf("oidc: invalid issuer: %w", err)
	}
	redirectURL, err := parseEndpointURL(cfg.GetRedirectUri())
	if err != nil {
		return nil, fmt.Errorf("oidc: invalid redirect_uri: %w", err)
	}
	if len(cfg.GetCookieSecret()) < minCookieSecretBytes {
		return nil, fmt.Errorf("oidc: cookie_secret must be at least %d bytes", minCookieSecretBytes)
	}
	if err := cfg.GetHttpTimeout().CheckValid(); err != nil || cfg.GetHttpTimeout().AsDuration() <= 0 {
		return nil, fmt.Errorf("oidc: http_timeout must be a positive duration")
	}

	hashKey, err := hkdf.Key(sha256.New, []byte(cfg.GetCookieSecret()), nil, hkdfHashInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("oidc: derive cookie hash key: %w", err)
	}
	encryptKey, err := hkdf.Key(sha256.New, []byte(cfg.GetCookieSecret()), nil, hkdfEncryptInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("oidc: derive cookie encrypt key: %w", err)
	}

	cookieOpts := []httphelper.CookieHandlerOpt{
		httphelper.WithMaxAge(handshakeCookieMaxAge),
		httphelper.WithPath("/"),
		httphelper.WithSameSite(http.SameSiteLaxMode),
	}
	baseName := sanitizeCookieToken(cfg.GetClientId()) + "_oidc_handshake"
	cookieName := "__Host-" + baseName
	if strings.EqualFold(redirectURL.Scheme, "http") {
		cookieOpts = append(cookieOpts, httphelper.WithUnsecure())
		cookieName = baseName
	}

	scopes := slices.Clone(cfg.GetScopes())
	if len(scopes) == 0 {
		scopes = []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}
	} else if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}

	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	timeout := cfg.GetHttpTimeout().AsDuration()
	httpClient := resolveHTTPClient(o.httpClient, http.Client{Timeout: timeout})

	return &RelyingParty{
		issuer:       strings.TrimRight(issuerURL.String(), "/"),
		clientID:     cfg.GetClientId(),
		clientSecret: cfg.GetClientSecret(),
		redirectURI:  redirectURL.String(),
		scopes:       scopes,
		cookieName:   cookieName,
		cookies:      httphelper.NewCookieHandler(hashKey, encryptKey, cookieOpts...),
		httpClient:   httpClient,
		timeout:      timeout,
	}, nil
}

// BeginLogin writes a 5-minute encrypted handshake cookie containing
// {state, nonce, code_verifier, return_to} and issues a 302 redirect to the OP.
func (p *RelyingParty) BeginLogin(w http.ResponseWriter, r *http.Request, defaultReturnTo string) error {
	if r == nil || w == nil {
		return fmt.Errorf("oidc: request and response writer are required")
	}
	relyingParty, err := p.discover(r.Context())
	if err != nil {
		return err
	}

	fallback := sanitizeReturnTo(defaultReturnTo, "/")
	returnTo := sanitizeReturnTo(r.URL.Query().Get("return_to"), fallback)

	state, err := randomURLToken(32)
	if err != nil {
		return fmt.Errorf("oidc: generate state: %w", err)
	}
	nonce, err := randomURLToken(32)
	if err != nil {
		return fmt.Errorf("oidc: generate nonce: %w", err)
	}
	codeVerifier, err := randomURLToken(32)
	if err != nil {
		return fmt.Errorf("oidc: generate code_verifier: %w", err)
	}

	encoded, err := json.Marshal(handshakeState{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		ReturnTo:     returnTo,
	})
	if err != nil {
		return fmt.Errorf("oidc: marshal handshake state: %w", err)
	}
	if err := p.cookies.SetCookie(w, p.cookieName, string(encoded)); err != nil {
		return fmt.Errorf("oidc: set handshake cookie: %w", err)
	}

	authURL := rp.AuthURL(
		state,
		relyingParty,
		rp.WithCodeChallenge(oidc.NewSHACodeChallenge(codeVerifier)),
		func() []oauth2.AuthCodeOption {
			return []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("nonce", nonce)}
		},
	)
	http.Redirect(w, r, authURL, http.StatusFound)
	return nil
}

// CompleteLogin consumes and deletes the one-time handshake cookie, validates
// state, exchanges the authorization code with PKCE S256, verifies ID token
// signature/iss/aud/exp/nonce/at_hash, and enriches profile/email claims.
func (p *RelyingParty) CompleteLogin(w http.ResponseWriter, r *http.Request) (*VerifiedIdentity, string, error) {
	if r == nil || w == nil {
		return nil, "", fmt.Errorf("oidc: request and response writer are required")
	}

	rawCookie, cookieErr := p.cookies.CheckCookie(r, p.cookieName)
	p.cookies.DeleteCookie(w, p.cookieName)

	if r.URL.Query().Get("error") != "" {
		return nil, "", fmt.Errorf("%w: upstream returned authorization error", ErrHandshakeInvalid)
	}
	if cookieErr != nil {
		return nil, "", fmt.Errorf("%w: missing, expired, or invalid handshake cookie", ErrHandshakeInvalid)
	}

	var hs handshakeState
	if err := json.Unmarshal([]byte(rawCookie), &hs); err != nil || hs.State == "" || hs.Nonce == "" || hs.CodeVerifier == "" {
		return nil, "", fmt.Errorf("%w: malformed handshake payload", ErrHandshakeInvalid)
	}

	queryState := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if code == "" || subtle.ConstantTimeCompare([]byte(queryState), []byte(hs.State)) != 1 {
		return nil, "", fmt.Errorf("%w: state mismatch or missing authorization code", ErrHandshakeInvalid)
	}

	relyingParty, err := p.discover(r.Context())
	if err != nil {
		return nil, "", err
	}

	ctx := withNonce(r.Context(), hs.Nonce)
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, relyingParty, rp.WithCodeVerifier(hs.CodeVerifier))
	if err != nil {
		if ctxErr := r.Context().Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", classifyExchangeError(err)
	}
	if tokens == nil || tokens.IDTokenClaims == nil {
		return nil, "", fmt.Errorf("%w: missing verified id_token claims", ErrHandshakeInvalid)
	}

	idClaims := tokens.IDTokenClaims
	if idClaims.GetSubject() == "" || subtle.ConstantTimeCompare([]byte(idClaims.GetNonce()), []byte(hs.Nonce)) != 1 {
		return nil, "", fmt.Errorf("%w: invalid subject or nonce claim", ErrHandshakeInvalid)
	}

	identity := &VerifiedIdentity{
		Subject:       idClaims.GetSubject(),
		Email:         idClaims.Email,
		EmailVerified: bool(idClaims.EmailVerified),
		Name:          idClaims.Name,
		Picture:       idClaims.Picture,
		AuthTime:      idClaims.GetAuthTime(),
	}

	needsUserinfo := (slices.Contains(p.scopes, oidc.ScopeEmail) && identity.Email == "") ||
		(slices.Contains(p.scopes, oidc.ScopeProfile) && identity.Name == "" && identity.Picture == "")
	if needsUserinfo && relyingParty.UserinfoEndpoint() != "" && tokens.AccessToken != "" {
		tokenType := tokens.TokenType
		if tokenType == "" {
			tokenType = oidc.BearerToken
		}
		info, err := rp.Userinfo[*oidc.UserInfo](r.Context(), tokens.AccessToken, tokenType, identity.Subject, relyingParty)
		if err != nil {
			if ctxErr := r.Context().Err(); ctxErr != nil {
				return nil, "", ctxErr
			}
			if errors.Is(err, rp.ErrUserInfoSubNotMatching) {
				return nil, "", fmt.Errorf("%w: userinfo subject mismatch", ErrHandshakeInvalid)
			}
			return nil, "", classifyExchangeError(err)
		}
		if info != nil {
			if identity.Email == "" {
				identity.Email = info.Email
				identity.EmailVerified = bool(info.EmailVerified)
			}
			if identity.Name == "" {
				identity.Name = info.Name
			}
			if identity.Picture == "" {
				identity.Picture = info.Picture
			}
		}
	}

	return identity, sanitizeReturnTo(hs.ReturnTo, "/"), nil
}

func (p *RelyingParty) discover(ctx context.Context) (rp.RelyingParty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cached := p.rp.Load(); cached != nil {
		return *cached, nil
	}

	ch := p.sf.DoChan("discovery", func() (any, error) {
		if cached := p.rp.Load(); cached != nil {
			return *cached, nil
		}
		discCtx, cancel := context.WithTimeout(context.Background(), p.timeout)
		defer cancel()

		discovered, err := rp.NewRelyingPartyOIDC(
			discCtx,
			p.issuer,
			p.clientID,
			p.clientSecret,
			p.redirectURI,
			p.scopes,
			rp.WithHTTPClient(p.httpClient),
			rp.WithAuthStyle(oauth2.AuthStyleInHeader),
			rp.WithSigningAlgsFromDiscovery(),
			rp.WithVerifierOpts(rp.WithNonce(nonceFromContext)),
		)
		if err != nil {
			return nil, fmt.Errorf("%w: discovery failed", ErrProviderUnavailable)
		}
		p.rp.Store(&discovered)
		return discovered, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(rp.RelyingParty), nil
	}
}

func classifyExchangeError(err error) error {
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.Response != nil && retrieveErr.Response.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: upstream token endpoint returned %d", ErrProviderUnavailable, retrieveErr.Response.StatusCode)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%w: upstream provider request failed", ErrProviderUnavailable)
	}
	return fmt.Errorf("%w: code exchange or token verification failed", ErrHandshakeInvalid)
}

func parseEndpointURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty URL")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return nil, fmt.Errorf("scheme must be http or https")
	}
	if u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("URL must not contain userinfo, query, or fragment")
	}
	return u, nil
}

func sanitizeReturnTo(candidate, fallback string) string {
	if isSafeRelativePath(candidate) {
		return candidate
	}
	if isSafeRelativePath(fallback) {
		return fallback
	}
	return "/"
}

func isSafeRelativePath(value string) bool {
	if value == "" || value[0] != '/' {
		return false
	}
	if strings.HasPrefix(value, "//") || strings.HasPrefix(value, `/\`) {
		return false
	}
	for i := range len(value) {
		ch := value[i]
		if ch < 0x20 || ch == 0x7f || ch == '\\' {
			return false
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil {
		return false
	}
	return true
}

func sanitizeCookieToken(raw string) string {
	var b strings.Builder
	for i := range len(raw) {
		ch := raw[i]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			b.WriteByte(ch)
		}
	}
	if b.Len() == 0 {
		return "app"
	}
	return b.String()
}

func randomURLToken(numBytes int) (string, error) {
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
