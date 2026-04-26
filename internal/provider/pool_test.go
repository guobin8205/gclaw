package provider

import (
	"errors"
	"testing"
	"time"
)

func TestPoolRoundRobin(t *testing.T) {
	pool := NewCredentialPool([]string{"key-a", "key-b", "key-c"}, time.Minute)

	got := make(map[string]int)
	for i := 0; i < 6; i++ {
		key, err := pool.Select()
		if err != nil {
			t.Fatalf("Select() %d: %v", i, err)
		}
		got[key]++
	}

	for _, k := range []string{"key-a", "key-b", "key-c"} {
		if got[k] != 2 {
			t.Errorf("expected %q selected 2 times, got %d", k, got[k])
		}
	}
}

func TestPoolSingleKey(t *testing.T) {
	pool := NewCredentialPool([]string{"only-key"}, time.Minute)

	key, err := pool.Select()
	if err != nil {
		t.Fatal(err)
	}
	if key != "only-key" {
		t.Errorf("expected only-key, got %q", key)
	}
}

func TestPoolExhausted(t *testing.T) {
	pool := NewCredentialPool([]string{"key-a", "key-b"}, time.Minute)

	// Exhaust key-a
	pool.MarkExhausted("key-a", errors.New("429"))

	// Should still get key-b
	key, err := pool.Select()
	if err != nil {
		t.Fatal(err)
	}
	if key != "key-b" {
		t.Errorf("expected key-b after key-a exhausted, got %q", key)
	}
}

func TestPoolAllExhausted(t *testing.T) {
	pool := NewCredentialPool([]string{"key-a", "key-b"}, time.Minute)

	pool.MarkExhausted("key-a", errors.New("429"))
	pool.MarkExhausted("key-b", errors.New("429"))

	_, err := pool.Select()
	if err == nil {
		t.Error("expected error when all keys exhausted")
	}
}

func TestPoolCooldownRecovery(t *testing.T) {
	pool := NewCredentialPool([]string{"key-a", "key-b"}, 50*time.Millisecond)

	pool.MarkExhausted("key-a", errors.New("429"))

	// key-a should be skipped
	key, _ := pool.Select()
	if key != "key-b" {
		t.Errorf("expected key-b, got %q", key)
	}

	// Wait for cooldown
	time.Sleep(60 * time.Millisecond)

	// key-a should recover
	key, err := pool.Select()
	if err != nil {
		t.Fatal(err)
	}
	if key != "key-a" {
		t.Errorf("expected key-a after cooldown, got %q", key)
	}
}

func TestPoolStatus(t *testing.T) {
	pool := NewCredentialPool([]string{"key-a", "key-b"}, time.Minute)

	pool.MarkExhausted("key-a", errors.New("429"))

	statuses := pool.Status()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Status != KeyExhausted {
		t.Error("expected key-a to be exhausted")
	}
	if statuses[0].FailCount != 1 {
		t.Errorf("expected fail count 1, got %d", statuses[0].FailCount)
	}
	if statuses[1].Status != KeyOK {
		t.Error("expected key-b to be OK")
	}
}

func TestPoolEmpty(t *testing.T) {
	pool := NewCredentialPool(nil, time.Minute)
	_, err := pool.Select()
	if err == nil {
		t.Error("expected error for empty pool")
	}
}

func TestPoolDefaultCooldown(t *testing.T) {
	pool := NewCredentialPool([]string{"key-a"}, 0)
	if pool.cooldown != 5*time.Minute {
		t.Errorf("expected default cooldown 5m, got %v", pool.cooldown)
	}
}
