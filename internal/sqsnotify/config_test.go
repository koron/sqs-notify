package sqsnotify

import (
	"testing"
	"time"
)

func TestNewConfigAutoExtendDefaults(t *testing.T) {
	cfg := NewConfig()
	if cfg == nil {
		t.Fatal("NewConfig() returned nil")
	}

	if cfg.AutoExtend != false {
		t.Errorf("expected AutoExtend = false, got %v", cfg.AutoExtend)
	}
	if cfg.AutoExtendFactor != 2.0 {
		t.Errorf("expected AutoExtendFactor = 2.0, got %v", cfg.AutoExtendFactor)
	}
	if cfg.AutoExtendMax != 64*time.Minute {
		t.Errorf("expected AutoExtendMax = 64m, got %v", cfg.AutoExtendMax)
	}
}
