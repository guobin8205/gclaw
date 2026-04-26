package provider

import (
	"errors"
	"sync"
	"time"
)

// KeyStatus represents the health state of a credential.
type KeyStatus int

const (
	KeyOK        KeyStatus = iota
	KeyExhausted           // rate-limited or quota exceeded
)

// PooledCredential tracks a single API key's state.
type PooledCredential struct {
	Key           string
	Status        KeyStatus
	LastUsed      time.Time
	FailCount     int
	LastError     error
	CooldownUntil time.Time
}

// CredentialPool manages round-robin rotation across multiple API keys
// with exhaustion tracking and automatic cooldown recovery.
type CredentialPool struct {
	creds    []PooledCredential
	index    int
	cooldown time.Duration
	mu       sync.RWMutex
}

// NewCredentialPool creates a pool from a list of API keys.
func NewCredentialPool(keys []string, cooldown time.Duration) *CredentialPool {
	creds := make([]PooledCredential, len(keys))
	for i, k := range keys {
		creds[i] = PooledCredential{Key: k}
	}
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	return &CredentialPool{
		creds:    creds,
		cooldown: cooldown,
	}
}

// Select returns the next healthy API key via round-robin.
// Skips exhausted keys that are still in cooldown.
func (p *CredentialPool) Select() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.recoverLocked()

	if len(p.creds) == 0 {
		return "", errors.New("credential pool is empty")
	}

	for i := 0; i < len(p.creds); i++ {
		idx := (p.index + i) % len(p.creds)
		c := &p.creds[idx]
		if c.Status == KeyOK || time.Now().After(c.CooldownUntil) {
			if c.Status == KeyExhausted {
				c.Status = KeyOK
				c.LastError = nil
			}
			c.LastUsed = time.Now()
			p.index = (idx + 1) % len(p.creds)
			return c.Key, nil
		}
	}

	return "", errors.New("all credentials exhausted or in cooldown")
}

// MarkExhausted marks a key as exhausted after a failure.
func (p *CredentialPool) MarkExhausted(key string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i := range p.creds {
		if p.creds[i].Key == key {
			p.creds[i].Status = KeyExhausted
			p.creds[i].FailCount++
			p.creds[i].LastError = err
			p.creds[i].CooldownUntil = time.Now().Add(p.cooldown)
			return
		}
	}
}

// Recover resets any keys whose cooldown has expired.
func (p *CredentialPool) Recover() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recoverLocked()
}

func (p *CredentialPool) recoverLocked() {
	now := time.Now()
	for i := range p.creds {
		if p.creds[i].Status == KeyExhausted && now.After(p.creds[i].CooldownUntil) {
			p.creds[i].Status = KeyOK
			p.creds[i].LastError = nil
		}
	}
}

// Status returns a snapshot of all credentials for diagnostics.
func (p *CredentialPool) Status() []PooledCredential {
	p.mu.RLock()
	defer p.mu.RUnlock()

	snapshot := make([]PooledCredential, len(p.creds))
	copy(snapshot, p.creds)
	return snapshot
}

// Len returns the number of credentials in the pool.
func (p *CredentialPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.creds)
}
