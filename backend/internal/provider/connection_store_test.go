package provider

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestStore creates a ConnectionStore backed by a temp directory and returns
// both the store and the expected path to connections.json.
func newTestStore(t *testing.T) (*ConnectionStore, string) {
	t.Helper()
	dir := t.TempDir()
	store := NewConnectionStore(dir)
	path := filepath.Join(dir, "connections.json")
	return store, path
}

// ollamaConn returns a valid Ollama connection suitable for test operations.
func ollamaConn() Connection {
	return Connection{
		Name:         "test-ollama",
		ProviderType: ProviderOllama,
		BaseURL:      "http://localhost:11434",
		DefaultModel: "llama3:8b",
	}
}

// apiKeyConn returns a valid OpenAI connection with an API key.
func apiKeyConn() Connection {
	return Connection{
		Name:         "test-openai",
		ProviderType: ProviderOpenAI,
		APIKey:       "sk-test-key",
		DefaultModel: "gpt-4o",
	}
}

// checkMode asserts that the file at path has exactly mode 0600.
func checkMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	got := info.Mode().Perm()
	if got != 0o600 {
		t.Errorf("want connections.json mode 0600, got %04o", got)
	}
}

// ---------------------------------------------------------------------------
// chmod 0600 verified after Create
// ---------------------------------------------------------------------------

func TestConnectionStore_Create_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	_, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		t.Fatalf("connections.json not created at %s", path)
	}
	checkMode(t, path)
}

// TestConnectionStore_Create_APIKey_FileMode0600 explicitly tests the sensitive
// case: a connection with an API key must still land in a 0600 file.
func TestConnectionStore_Create_APIKey_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	_, err := store.Create(apiKeyConn())
	if err != nil {
		t.Fatalf("Create with API key: %v", err)
	}

	checkMode(t, path)
}

// TestConnectionStore_Create_Multiple_FileMode0600 creates several connections
// in sequence and verifies the file mode is still 0600 after each write.
func TestConnectionStore_Create_Multiple_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	for i := 0; i < 3; i++ {
		conn := ollamaConn()
		conn.Name = conn.Name + string(rune('A'+i)) // unique names
		if _, err := store.Create(conn); err != nil {
			t.Fatalf("Create #%d: %v", i+1, err)
		}
		checkMode(t, path)
	}
}

// ---------------------------------------------------------------------------
// chmod 0600 verified after Update
// ---------------------------------------------------------------------------

func TestConnectionStore_Update_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	created, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := *created
	updated.DefaultModel = "llama3:70b"
	if _, err := store.Update(updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	checkMode(t, path)
}

// TestConnectionStore_Update_AddAPIKey_FileMode0600 adds an API key during
// update (most security-sensitive write path).
func TestConnectionStore_Update_AddAPIKey_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	created, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Simulate upgrading the connection type to OpenAI with an API key.
	updated := *created
	updated.ProviderType = ProviderOpenAI
	updated.BaseURL = "" // no longer required
	updated.APIKey = "sk-newly-added-key"
	updated.DefaultModel = "gpt-4o"
	if _, err := store.Update(updated); err != nil {
		t.Fatalf("Update with API key: %v", err)
	}

	checkMode(t, path)
}

// ---------------------------------------------------------------------------
// chmod 0600 verified after Delete
// ---------------------------------------------------------------------------

func TestConnectionStore_Delete_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	// Create two connections so the file remains non-empty after one is deleted.
	first, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}

	second := ollamaConn()
	second.Name = "test-ollama-2"
	if _, err := store.Create(second); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	if _, err := store.Delete(first.ID, nil, nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	checkMode(t, path)
}

// TestConnectionStore_Delete_LastConnection_FileMode0600 deletes the only
// remaining connection, leaving an empty array in the file.
func TestConnectionStore_Delete_LastConnection_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	created, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := store.Delete(created.ID, nil, nil); err != nil {
		t.Fatalf("Delete last connection: %v", err)
	}

	checkMode(t, path)
}

// TestConnectionStore_Delete_WithAPIKey_FileMode0600 specifically covers the
// case where the remaining connections contain an API key.
func TestConnectionStore_Delete_WithAPIKey_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	toDelete, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create ollama: %v", err)
	}
	if _, err := store.Create(apiKeyConn()); err != nil {
		t.Fatalf("Create openai: %v", err)
	}

	if _, err := store.Delete(toDelete.ID, nil, nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// File still contains the OpenAI connection with an API key — must be 0600.
	checkMode(t, path)
}

// ---------------------------------------------------------------------------
// chmod 0600 is idempotent across a full Create → Update → Delete lifecycle
// ---------------------------------------------------------------------------

func TestConnectionStore_FullLifecycle_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	// Create.
	created, err := store.Create(ollamaConn())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	checkMode(t, path)

	// Update.
	updated := *created
	updated.DefaultModel = "llama3:70b"
	updated2, err := store.Update(updated)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	checkMode(t, path)

	// Verify data integrity alongside permission check.
	if updated2.DefaultModel != "llama3:70b" {
		t.Errorf("want defaultModel llama3:70b after update, got %s", updated2.DefaultModel)
	}

	// Delete.
	if _, err := store.Delete(updated2.ID, nil, nil); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	checkMode(t, path)
}

// ---------------------------------------------------------------------------
// Concurrent mutations preserve file integrity and permissions
// ---------------------------------------------------------------------------

func TestConnectionStore_ConcurrentCreates_FileMode0600(t *testing.T) {
	store, path := newTestStore(t)

	// Launch several goroutines that all call Create concurrently.
	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			conn := ollamaConn()
			conn.Name = conn.Name + string(rune('A'+i))
			_, err := store.Create(conn)
			errs <- err
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Create: %v", err)
		}
	}

	// After all goroutines finish, permissions must still be 0600.
	checkMode(t, path)

	// And the in-memory state must be consistent with what's on disk.
	list := store.List()
	if len(list) != n {
		t.Errorf("want %d connections after concurrent creates, got %d", n, len(list))
	}
}
