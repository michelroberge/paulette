package provider

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// BuildPool manages a set of connection slots for parallel bead execution.
// Each slot has a buffered semaphore channel limiting concurrent usage.
// Acquire selects across all non-degraded slots to get the first available.
type BuildPool struct {
	mu       sync.RWMutex
	slots    []poolSlot
	registry *Registry
}

type poolSlot struct {
	config   BuildPoolSlot
	sem      chan struct{}
	provider Provider
	model    string
	degraded atomic.Bool
	// Stats
	completed atomic.Int64
	active    atomic.Int64
}

// PoolLease is returned by Acquire. The caller must call Release when done.
type PoolLease struct {
	Provider   Provider
	Model      string
	Assignment *StageAssignment
	slotIdx    int
	pool       *BuildPool
}

// PoolSlotStatus is the runtime status of a single pool slot.
type PoolSlotStatus struct {
	ConnectionID   string `json:"connectionId"`
	Model          string `json:"model"`
	MaxParallel    int    `json:"maxParallel"`
	ActiveCount    int    `json:"activeCount"`
	Degraded       bool   `json:"degraded"`
	TotalCompleted int    `json:"totalCompleted"`
}

// NewBuildPool creates a pool from the given config and provider registry.
// Returns nil if cfg is nil or has no slots.
func NewBuildPool(cfg *BuildPoolConfig, registry *Registry) *BuildPool {
	if cfg == nil || len(cfg.Slots) == 0 {
		return nil
	}
	p := &BuildPool{registry: registry}
	p.configure(cfg.Slots)
	return p
}

func (p *BuildPool) configure(slots []BuildPoolSlot) {
	p.slots = make([]poolSlot, len(slots))
	for i, s := range slots {
		maxP := s.MaxParallel
		if maxP < 1 {
			maxP = 1
		}
		if maxP > 10 {
			maxP = 10
		}
		prov, err := p.registry.ForConnection(s.ConnectionID)
		if err != nil {
			// Slot starts degraded if connection can't be resolved
			p.slots[i] = poolSlot{
				config: s,
				sem:    make(chan struct{}, maxP),
				model:  s.Model,
			}
			p.slots[i].degraded.Store(true)
			continue
		}
		p.slots[i] = poolSlot{
			config:   s,
			sem:      make(chan struct{}, maxP),
			provider: prov,
			model:    s.Model,
		}
	}
}

// Acquire blocks until a slot is available or ctx is cancelled.
// Uses reflect.Select to fairly choose among all non-degraded slots.
func (p *BuildPool) Acquire(ctx context.Context) (*PoolLease, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.slots) == 0 {
		return nil, fmt.Errorf("build pool has no slots")
	}

	// Build select cases: ctx.Done() first, then all non-degraded slot semaphores.
	type slotRef struct{ idx int }
	var refs []slotRef
	cases := []reflect.SelectCase{
		{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())},
	}

	for i := range p.slots {
		if p.slots[i].degraded.Load() || p.slots[i].provider == nil {
			continue
		}
		cases = append(cases, reflect.SelectCase{
			Dir:  reflect.SelectSend,
			Chan: reflect.ValueOf(p.slots[i].sem),
			Send: reflect.ValueOf(struct{}{}),
		})
		refs = append(refs, slotRef{idx: i})
	}

	if len(refs) == 0 {
		return nil, fmt.Errorf("all build pool slots are degraded")
	}

	chosen, _, _ := reflect.Select(cases)
	if chosen == 0 {
		return nil, ctx.Err()
	}

	slotIdx := refs[chosen-1].idx
	slot := &p.slots[slotIdx]
	slot.active.Add(1)

	return &PoolLease{
		Provider: slot.provider,
		Model:    slot.model,
		Assignment: &StageAssignment{
			ConnectionID: slot.config.ConnectionID,
			Model:        slot.model,
		},
		slotIdx: slotIdx,
		pool:    p,
	}, nil
}

// Release returns a lease to the pool.
func (p *BuildPool) Release(lease *PoolLease) {
	if lease == nil || lease.pool != p {
		return
	}
	p.mu.RLock()
	defer p.mu.RUnlock()

	if lease.slotIdx >= len(p.slots) {
		return
	}
	slot := &p.slots[lease.slotIdx]
	slot.active.Add(-1)
	slot.completed.Add(1)
	<-slot.sem
}

// MarkDegraded marks a slot as degraded for the given duration.
func (p *BuildPool) MarkDegraded(slotIdx int, duration time.Duration) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if slotIdx >= len(p.slots) {
		return
	}
	p.slots[slotIdx].degraded.Store(true)
	go func() {
		time.Sleep(duration)
		p.mu.RLock()
		defer p.mu.RUnlock()
		if slotIdx < len(p.slots) {
			p.slots[slotIdx].degraded.Store(false)
		}
	}()
}

// Reconfigure replaces the pool slots with a new configuration.
// Active leases continue to use their existing slot; new Acquire calls
// use the new configuration.
func (p *BuildPool) Reconfigure(cfg BuildPoolConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.configure(cfg.Slots)
}

// Status returns the runtime status of all slots.
func (p *BuildPool) Status() []PoolSlotStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]PoolSlotStatus, len(p.slots))
	for i := range p.slots {
		s := &p.slots[i]
		result[i] = PoolSlotStatus{
			ConnectionID:   s.config.ConnectionID,
			Model:          s.model,
			MaxParallel:    s.config.MaxParallel,
			ActiveCount:    int(s.active.Load()),
			Degraded:       s.degraded.Load(),
			TotalCompleted: int(s.completed.Load()),
		}
	}
	return result
}

// Active returns true if the pool has any non-degraded slots configured.
func (p *BuildPool) Active() bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	for i := range p.slots {
		if !p.slots[i].degraded.Load() && p.slots[i].provider != nil {
			return true
		}
	}
	return false
}
