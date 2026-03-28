package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
	contentTypeSSE    = "text/event-stream"

	claudeClientID    = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeTokenURL    = "https://platform.claude.com/v1/oauth/token"
	claudeRedirectURI = "https://platform.claude.com/oauth/code/callback"
	claudeAuthBase    = "https://claude.ai/oauth/authorize"
	claudeScope       = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
)

// pkceSession holds an in-progress OAuth PKCE login flow.
// It lives independently of any SSE connection so reconnecting clients
// can replay the auth URL.
type pkceSession struct {
	authURL      string
	codeVerifier string
	state        string

	mu   sync.Mutex
	done bool
	err  string
	ch   chan struct{} // closed each time state changes
}

func newPKCESession() (*pkceSession, error) {
	verifier, err := randomBase64URL(32)
	if err != nil {
		return nil, err
	}
	state, err := randomBase64URL(32)
	if err != nil {
		return nil, err
	}
	challenge := pkceChallenge(verifier)

	params := url.Values{}
	params.Set("code", "true") // signals headless / manual-redirect flow
	params.Set("client_id", claudeClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", claudeRedirectURI)
	params.Set("scope", claudeScope)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)

	return &pkceSession{
		authURL:      claudeAuthBase + "?" + params.Encode(),
		codeVerifier: verifier,
		state:        state,
		ch:           make(chan struct{}),
	}, nil
}

func (s *pkceSession) finish(errMsg string) {
	s.mu.Lock()
	s.done = true
	s.err = errMsg
	ch := s.ch
	s.ch = make(chan struct{})
	s.mu.Unlock()
	if errMsg != "" {
		fmt.Printf("[auth] session finished with error: %q\n", errMsg)
	} else {
		fmt.Printf("[auth] session finished successfully\n")
	}
	close(ch)
}

func (s *pkceSession) wait() (done bool, errMsg string, ch chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done, s.err, s.ch
}

// AuthHandler handles Claude OAuth authentication.
// It implements its own PKCE flow so that the token exchange runs inside the
// container (no subprocess), avoiding the need for claude CLI to be installed
// on the host machine.
type AuthHandler struct {
	mu      sync.Mutex
	session *pkceSession
}

// NewAuthHandler constructs an AuthHandler.  claudePath is accepted for
// interface compatibility but no longer used.
func NewAuthHandler(_ string) *AuthHandler {
	return &AuthHandler{}
}

// authStatus is the JSON shape returned by GET /api/auth/status.
type authStatus struct {
	Authenticated bool   `json:"authenticated"`
	Account       string `json:"account,omitempty"`
}

// Status checks whether Claude credentials are available.
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := h.checkStatus()
	fmt.Printf("[auth] status: authenticated=%v account=%q\n", status.Authenticated, status.Account)
	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(status)
}

func (h *AuthHandler) checkStatus() authStatus {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return authStatus{Authenticated: true, Account: "api-key"}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return authStatus{}
	}

	creds := filepath.Join(home, ".claude", ".credentials.json")
	fmt.Printf("Claude credentials path: %s\n", creds)

	data, err := os.ReadFile(creds)
	if err != nil {
		return authStatus{}
	}
	var raw claudeCredentials
	if err := json.Unmarshal(data, &raw); err != nil || raw.ClaudeAiOauth == nil {
		return authStatus{}
	}
	if raw.ClaudeAiOauth.AccessToken == "" {
		return authStatus{}
	}
	account := raw.ClaudeAiOauth.SubscriptionType // e.g. "max", "pro"
	return authStatus{Authenticated: true, Account: account}
}

func extractAccount(raw map[string]any) string {
	for _, key := range []string{"emailAddress", "email", "account", "username"} {
		if v, ok := raw[key].(string); ok && v != "" {
			return v
		}
	}
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			if acct := extractAccount(m); acct != "" {
				return acct
			}
		}
	}
	return ""
}

// Login streams the OAuth auth URL as Server-Sent Events then waits for the
// exchange to complete.  Reconnecting clients get the same URL replayed.
//
// SSE events:
//
//	data: {"type":"output","line":"..."}   — informational line (auth URL)
//	data: {"type":"done"}                  — exchange succeeded
//	data: {"type":"error","line":"..."}    — exchange failed
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set(headerContentType, contentTypeSSE)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(eventType, payload string) {
		data, _ := json.Marshal(map[string]string{"type": eventType, "line": payload})
		fmt.Printf("[auth] login event: type=%s\n", eventType)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	h.mu.Lock()
	if h.session == nil {
		sess, err := newPKCESession()
		if err != nil {
			h.mu.Unlock()
			http.Error(w, "failed to start auth session: "+err.Error(), http.StatusInternalServerError)
			return
		}
		h.session = sess
		fmt.Printf("[auth] new PKCE session started\n")
	}
	sess := h.session
	h.mu.Unlock()

	// Always send the auth URL so reconnecting clients see it.
	send("output", "If the browser didn't open, visit: "+sess.authURL)

	// Wait for the exchange to complete (or client disconnect).
	for {
		done, errMsg, ch := sess.wait()
		if done {
			if errMsg != "" {
				send("error", errMsg)
			} else {
				send("done", "")
			}
			return
		}
		select {
		case <-ch:
			// state changed — loop and check again
		case <-r.Context().Done():
			return
		}
	}
}

// tokenResponse is the JSON body returned by the OAuth token endpoint.
type tokenResponse struct {
	AccessToken      string  `json:"access_token"`
	RefreshToken     string  `json:"refresh_token"`
	ExpiresIn        float64 `json:"expires_in"`
	Scope            string  `json:"scope"`
	SubscriptionType string  `json:"subscription_type"`
	RateLimitTier    string  `json:"rate_limit_tier"`
	Error            *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// LoginInput receives the code pasted by the user from platform.claude.com.
// The expected format is "CODE#STATE" as displayed on the callback page.
func (h *AuthHandler) LoginInput(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil || len(body) == 0 {
		http.Error(w, "missing input", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	sess := h.session
	h.mu.Unlock()

	input := strings.TrimSpace(string(body))
	fmt.Printf("[auth] LoginInput: input=%q sess=%v\n", input, sess != nil)

	if sess == nil {
		http.Error(w, "no active login session — please reopen the login panel", http.StatusConflict)
		return
	}

	sess.mu.Lock()
	alreadyDone := sess.done
	sess.mu.Unlock()
	if alreadyDone {
		http.Error(w, "login session already finished — please reopen the login panel", http.StatusConflict)
		return
	}

	// Parse the pasted input — accept two forms:
	//   1. Full redirect URL "https://...?code=X&state=Y" — user copied from browser address bar
	//   2. "CODE#STATE" or bare "CODE" — code shown on callback page
	var code, state string
	if strings.HasPrefix(input, "http") {
		if u, err := url.Parse(input); err == nil {
			code = u.Query().Get("code")
			state = u.Query().Get("state")
			if state == "" {
				state = sess.state
			}
		}
	}
	if code == "" {
		var ok bool
		code, state, ok = strings.Cut(input, "#")
		if !ok || code == "" {
			code = input
			state = sess.state
		}
	}

	fmt.Printf("[auth] LoginInput: code=%q state=%q expectedState=%q\n", code, state, sess.state)

	if state != sess.state {
		fmt.Printf("[auth] LoginInput: state mismatch — rejecting\n")
		http.Error(w, "state mismatch — please restart the login flow", http.StatusBadRequest)
		return
	}

	// Do the token exchange in the background so we can return 204 immediately.
	go func() {
		if err := h.exchangeToken(sess, code); err != nil {
			fmt.Printf("[auth] token exchange failed: %v\n", err)
			sess.finish(err.Error())
		} else {
			h.mu.Lock()
			if h.session == sess {
				h.session = nil
			}
			h.mu.Unlock()
			sess.finish("")
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}

// exchangeToken POSTs to the Claude OAuth token endpoint and writes the
// resulting credentials to ~/.claude/.credentials.json.
func (h *AuthHandler) exchangeToken(sess *pkceSession, code string) error {
	payload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     claudeClientID,
		"code":          code,
		"code_verifier": sess.codeVerifier,
		"redirect_uri":  claudeRedirectURI,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	fmt.Printf("[auth] exchangeToken: POST %s code=%q\n", claudeTokenURL, code)

	req, err := http.NewRequest(http.MethodPost, claudeTokenURL, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	fmt.Printf("[auth] exchangeToken: status=%s body=%s\n", resp.Status, string(respBody))

	var tok tokenResponse
	if err := json.Unmarshal(respBody, &tok); err != nil {
		return fmt.Errorf("parse response (status %s): %w", resp.Status, err)
	}

	if tok.Error != nil {
		return fmt.Errorf("token error %s: %s", tok.Error.Type, tok.Error.Message)
	}
	if tok.AccessToken == "" {
		return fmt.Errorf("empty access_token in response (status %s)", resp.Status)
	}

	return h.writeCredentials(tok)
}

// claudeCredentials is the shape of ~/.claude/.credentials.json.
type claudeCredentials struct {
	ClaudeAiOauth *claudeOAuthToken `json:"claudeAiOauth"`
}

type claudeOAuthToken struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"` // ms since epoch
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscriptionType,omitempty"`
	RateLimitTier    string   `json:"rateLimitTier,omitempty"`
}

func (h *AuthHandler) writeCredentials(tok tokenResponse) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	dir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir .claude: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).UnixMilli()
	if tok.ExpiresIn == 0 {
		expiresAt = time.Now().Add(time.Hour).UnixMilli() // sensible default
	}

	scopes := strings.Fields(tok.Scope)
	if len(scopes) == 0 {
		scopes = strings.Fields(claudeScope)
	}

	creds := claudeCredentials{
		ClaudeAiOauth: &claudeOAuthToken{
			AccessToken:      tok.AccessToken,
			RefreshToken:     tok.RefreshToken,
			ExpiresAt:        expiresAt,
			Scopes:           scopes,
			SubscriptionType: tok.SubscriptionType,
			RateLimitTier:    tok.RateLimitTier,
		},
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	credsPath := filepath.Join(dir, ".credentials.json")
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	fmt.Printf("[auth] credentials written to %s\n", credsPath)
	return nil
}

// Logout removes stored credentials and clears any active login session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.session = nil
	h.mu.Unlock()

	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, ".claude", ".credentials.json"))
	}

	w.Header().Set(headerContentType, contentTypeJSON)
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ── PKCE helpers ──────────────────────────────────────────────────────────────

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
