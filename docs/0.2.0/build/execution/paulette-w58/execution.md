The fix is straightforward — the `loadFromDisk` comment on lines 227-228 says the opposite of what's true. The function is always called while the lock is held (by `ReloadFromDisk`) or before any lock is needed (in `NewConnectionStore`). The comment should clarify that `loadFromDisk` does **not** acquire the lock itself, and that the caller is responsible for holding the write lock when appropriate.Done. The contradictory comment on `loadFromDisk` (lines 227–228) has been corrected from:

> *"IMPORTANT: Must NOT be called while s.mu is held; callers that need the lock must acquire it themselves (see ReloadFromDisk)."*

to:

> *"Does not acquire s.mu; the caller is responsible for holding the write lock when concurrent access is possible (see ReloadFromDisk)."*

This accurately describes the actual contract: `loadFromDisk` is a lock-free internal helper that relies on its callers to provide the appropriate synchronisation. `ReloadFromDisk` acquires the write lock and then calls it — which is the correct, intended usage pattern. The old comment falsely implied the opposite relationship, which would have confused any future developer reading it.