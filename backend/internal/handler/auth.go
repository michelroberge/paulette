package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// AuthHandler handles Claude CLI authentication status and login flows.
type AuthHandler struct {
	claudePath string
	mu         sync.Mutex
	loginStdin io.WriteCloser
}

func NewAuthHandler(claudePath string) *AuthHandler {
	return &AuthHandler{claudePath: claudePath}
}

// authStatus is the JSON shape returned by GET /api/auth/status.
type authStatus struct {
	Authenticated bool   `json:"authenticated"`
	Account       string `json:"account,omitempty"`
}

// Status checks whether the claude CLI is authenticated by inspecting
// ~/.claude/.credentials.json.  It also accepts a fallback of ANTHROPIC_API_KEY.
func (h *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	status := h.checkStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (h *AuthHandler) checkStatus() authStatus {
	// Fast-path: ANTHROPIC_API_KEY is always usable.
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return authStatus{Authenticated: true, Account: "api-key"}
	}

	// Check for OAuth credentials stored by the claude CLI.
	home, err := os.UserHomeDir()
	if err != nil {
		return authStatus{}
	}
	creds := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(creds)
	if err != nil {
		return authStatus{}
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || len(raw) == 0 {
		return authStatus{}
	}

	// Try to extract an account identifier from known credential fields.
	account := extractAccount(raw)
	return authStatus{Authenticated: true, Account: account}
}

func extractAccount(raw map[string]any) string {
	for _, key := range []string{"emailAddress", "email", "account", "username"} {
		if v, ok := raw[key].(string); ok && v != "" {
			return v
		}
	}
	// Walk one level deep into nested objects.
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			if acct := extractAccount(m); acct != "" {
				return acct
			}
		}
	}
	return ""
}

// Login starts a claude auth login subprocess and streams its output as
// Server-Sent Events so the frontend can display the OAuth URL and
// any instructions the CLI prints.
//
// The SSE stream sends events of the form:
//
//	data: {"type":"output","line":"..."}   — a line from claude stdout/stderr
//	data: {"type":"done"}                  — subprocess exited (auth complete or failed)
//	data: {"type":"error","message":"..."}
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	send := func(eventType, payload string) {
		data, _ := json.Marshal(map[string]string{"type": eventType, "line": payload})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	claudeBin := h.claudePath
	if claudeBin == "" {
		claudeBin = "claude"
	}

	// Run: claude auth login
	// The CLI will print an OAuth URL and wait for the browser callback.
	// In a container the browser cannot open, so the user must copy the URL
	// from the output below and open it manually.
	cmd := exec.CommandContext(r.Context(), claudeBin, "auth", "login")
	cmd.Env = append(os.Environ(), "BROWSER=echo") // prevents the CLI from trying to open a GUI browser

	stdin, err := cmd.StdinPipe()
	if err != nil {
		send("error", "could not start auth: "+err.Error())
		return
	}
	h.mu.Lock()
	h.loginStdin = stdin
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.loginStdin = nil
		h.mu.Unlock()
		stdin.Close()
	}()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		send("error", "could not start auth: "+err.Error())
		return
	}
	cmd.Stderr = cmd.Stdout // merge stderr into the same pipe

	if err := cmd.Start(); err != nil {
		send("error", "could not start auth: "+err.Error())
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			send("output", stripANSI(line))
		}
		// If the client disconnects, stop the subprocess.
		select {
		case <-r.Context().Done():
			cmd.Process.Kill()
			return
		default:
		}
	}

	if err := cmd.Wait(); err != nil {
		send("error", err.Error())
		return
	}

	// Emit done — frontend will re-poll /api/auth/status.
	doneData, _ := json.Marshal(map[string]string{"type": "done"})
	fmt.Fprintf(w, "data: %s\n\n", doneData)
	flusher.Flush()
}

// LoginInput sends a line of input to the running login subprocess (e.g. the
// authorization code the user copies from the browser).
func (h *AuthHandler) LoginInput(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 512))
	if err != nil || len(body) == 0 {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	stdin := h.loginStdin
	h.mu.Unlock()
	if stdin == nil {
		http.Error(w, "no active login session", http.StatusConflict)
		return
	}
	stdin.Write(append(bytes.TrimSpace(body), '\n'))
	w.WriteHeader(http.StatusNoContent)
}

// Logout runs claude auth logout to clear stored credentials.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claudeBin := h.claudePath
	if claudeBin == "" {
		claudeBin = "claude"
	}

	out, err := exec.CommandContext(r.Context(), claudeBin, "auth", "logout").CombinedOutput()
	if err != nil {
		// Log but don't fail — credentials file may not exist.
		_ = out
	}

	// Also remove the credentials file directly in case the CLI command fails.
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, ".claude", ".credentials.json"))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// stripANSI removes ANSI escape codes from a string so raw terminal output
// is readable when sent to the browser.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1B && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			if i < len(s) {
				i++ // skip 'm'
			}
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
