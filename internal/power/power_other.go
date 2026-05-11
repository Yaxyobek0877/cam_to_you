//go:build !windows

// Package power — Windows'dan tashqari platformalarda no-op.
package power

import "sync"

type Keeper struct {
	mu    sync.Mutex
	count int
}

func New() *Keeper { return &Keeper{} }

func (k *Keeper) Hold() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.count++
	return nil
}

func (k *Keeper) Release() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.count > 0 {
		k.count--
	}
	return nil
}

func (k *Keeper) IsHeld() bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.count > 0
}
