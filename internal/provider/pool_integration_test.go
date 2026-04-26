package provider

import (
	"errors"
	"testing"
	"time"
)

// --- 凭证池集成验证 ---

func TestPoolIntegration_FullRotationCycle(t *testing.T) {
	// 场景：3 个 key，模拟完整轮转 + exhausted + 恢复
	pool := NewCredentialPool([]string{"key-1", "key-2", "key-3"}, 100*time.Millisecond)

	// 第 1 轮：正常 round-robin
	keys := make([]string, 3)
	for i := range keys {
		k, err := pool.Select()
		if err != nil {
			t.Fatalf("round 1 select %d: %v", i, err)
		}
		keys[i] = k
	}
	if keys[0] != "key-1" || keys[1] != "key-2" || keys[2] != "key-3" {
		t.Errorf("round 1 order wrong: %v", keys)
	}

	// 标记 key-1 和 key-2 为 exhausted
	pool.MarkExhausted("key-1", errors.New("429 rate limit"))
	pool.MarkExhausted("key-2", errors.New("429 rate limit"))

	// 验证：只剩 key-3 可用
	k, err := pool.Select()
	if err != nil {
		t.Fatal(err)
	}
	if k != "key-3" {
		t.Errorf("expected key-3 (only healthy), got %q", k)
	}

	// 标记 key-3 也 exhausted
	pool.MarkExhausted("key-3", errors.New("429 rate limit"))

	// 全挂
	_, err = pool.Select()
	if err == nil {
		t.Error("expected error when all keys exhausted")
	}

	// 等冷却恢复
	time.Sleep(120 * time.Millisecond)

	// 恢复后应该能再次获取 key
	k, err = pool.Select()
	if err != nil {
		t.Fatalf("after cooldown: %v", err)
	}
	t.Logf("after cooldown got key: %q", k)

	// 状态检查
	statuses := pool.Status()
	for _, s := range statuses {
		if s.Status != KeyOK {
			t.Errorf("key %q should be OK after cooldown, got status=%d", s.Key, s.Status)
		}
		if s.FailCount != 1 {
			t.Errorf("key %q should have FailCount=1, got %d", s.Key, s.FailCount)
		}
	}
}

func TestPoolIntegration_FactoryMultiKey(t *testing.T) {
	// 验证 Factory 的多 key 支持
	f := NewFactory()
	f.Register("test-multi", Config{
		Type:  "mock",
		Model: "test-model",
		Keys:  []string{"key-a", "key-b"},
	})

	// Build 应该成功（mock 不用 key，但池应该被创建）
	m, pool, err := f.BuildWithPool("test-multi")
	if err != nil {
		t.Fatal(err)
	}
	if m == nil {
		t.Error("expected non-nil model")
	}
	if pool == nil {
		t.Error("expected credential pool for multi-key config")
	}
	if pool.Len() != 2 {
		t.Errorf("expected pool size 2, got %d", pool.Len())
	}
}

func TestPoolIntegration_FactorySingleKeyNoPool(t *testing.T) {
	// 单 key 不应该创建池
	f := NewFactory()
	f.Register("test-single", Config{
		Type:   "mock",
		Model:  "test-model",
		APIKey: "single-key",
	})

	_, pool, err := f.BuildWithPool("test-single")
	if err != nil {
		t.Fatal(err)
	}
	if pool != nil {
		t.Error("single key should not create a pool")
	}
}

func TestPoolIntegration_MarkExhaustedViaPool(t *testing.T) {
	pool := NewCredentialPool([]string{"a", "b", "c"}, time.Minute)

	// 选中 a，然后标记 exhausted
	k1, _ := pool.Select()
	pool.MarkExhausted(k1, errors.New("429"))

	// 选中 b
	k2, _ := pool.Select()
	if k2 == k1 {
		t.Errorf("should not select exhausted key %q", k1)
	}

	// 选中 c
	k3, _ := pool.Select()
	if k3 == k1 || k3 == k2 {
		t.Errorf("should not select used keys, got %q", k3)
	}
}
