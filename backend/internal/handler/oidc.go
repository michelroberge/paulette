package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/michelroberge/paulette/backend/internal/config"
	"github.com/michelroberge/paulette/backend/internal/middleware"
	"golang.org/x/oauth2"
)

// stateEntry tracks a pending OIDC login state param and PKCE verifier.
type stateEntry struct {
	createdAt    time.Time
	codeVerifier string // PKCE code_verifier, sent during token exchange
}

// OIDCHandler handles OIDC login, callback, logout, and session info.
type OIDCHandler struct {
	cfg                *config.OIDCConfig
	oauth2Config       oauth2.Config
	verifier           *gooidc.IDTokenVerifier
	endSessionEndpoint string // from OIDC discovery, may be empty
	states             sync.Map // map[string]stateEntry
}

// NewOIDCHandler initialises the OIDC provider via discovery and returns the handler.
func NewOIDCHandler(cfg *config.OIDCConfig) (*OIDCHandler, error) {
	provider, err := gooidc.NewProvider(context.Background(), cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc provider discovery: %w", err)
	}

	oauth2Cfg := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURI,
		Endpoint:     provider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})

	// Extract end_session_endpoint from the provider's discovery document.
	var providerClaims struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	_ = provider.Claims(&providerClaims)

	return &OIDCHandler{
		cfg:                cfg,
		oauth2Config:       oauth2Cfg,
		verifier:           verifier,
		endSessionEndpoint: providerClaims.EndSessionEndpoint,
	}, nil
}

// Login generates a random state param and PKCE verifier, stores them,
// then redirects to the IDP with a S256 code challenge.
func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()
	h.states.Store(state, stateEntry{createdAt: time.Now(), codeVerifier: verifier})
	http.Redirect(w, r, h.oauth2Config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

// Callback validates the state, exchanges the code, creates a session cookie,
// and redirects the user to the app root.
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

	// Validate state (CSRF protection).
	raw, ok := h.states.LoadAndDelete(state)
	if !ok {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	entry := raw.(stateEntry)
	if time.Since(entry.createdAt) > 10*time.Minute {
		http.Error(w, "state expired", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens, passing the PKCE verifier.
	token, err := h.oauth2Config.Exchange(r.Context(), code, oauth2.VerifierOption(entry.codeVerifier))
	if err != nil {
		http.Error(w, "code exchange failed", http.StatusInternalServerError)
		return
	}

	// Extract and verify the ID token.
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusInternalServerError)
		return
	}
	idToken, err := h.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "id_token verification failed", http.StatusInternalServerError)
		return
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "failed to parse id_token claims", http.StatusInternalServerError)
		return
	}

	// Build a signed session JWT.
	sessionClaims := middleware.SessionClaims{
		Sub:   claims.Sub,
		Email: claims.Email,
		Name:  claims.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, sessionClaims).
		SignedString([]byte(h.cfg.SessionSecret))
	if err != nil {
		http.Error(w, "failed to sign session token", http.StatusInternalServerError)
		return
	}

	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{
		Name:     "paulette_session",
		Value:    signed,
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// Logout clears the session cookie and redirects to the IDP's end-session
// endpoint so the user is also logged out of the identity provider.
// If no end_session_endpoint is available, redirects to the app root.
func (h *OIDCHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "paulette_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	if h.endSessionEndpoint != "" {
		// Derive the app root from the redirect URI for post-logout redirect.
		appRoot := h.cfg.RedirectURI
		if u, err := url.Parse(h.cfg.RedirectURI); err == nil {
			appRoot = u.Scheme + "://" + u.Host
		}
		logoutURL := h.endSessionEndpoint +
			"?client_id=" + url.QueryEscape(h.cfg.ClientID) +
			"&post_logout_redirect_uri=" + url.QueryEscape(appRoot)
		http.Redirect(w, r, logoutURL, http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// Me parses the session cookie directly (since /api/auth/* is exempt from the
// auth middleware) and returns the user claims as JSON.
func (h *OIDCHandler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("paulette_session")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"}) //nolint:errcheck
		return
	}

	claims := &middleware.SessionClaims{}
	token, err := jwt.ParseWithClaims(cookie.Value, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(h.cfg.SessionSecret), nil
	})
	if err != nil || !token.Valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"}) //nolint:errcheck
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"sub":   claims.Sub,
		"email": claims.Email,
		"name":  claims.Name,
	})
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
