package main

import (
	"testing"
	"time"
)

func TestParseFlagsAutoExtend(t *testing.T) {
	t.Run("default flags", func(t *testing.T) {
		args := []string{"-queue", "test-queue", "echo", "hello"}
		params, err := parseFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.cfg.AutoExtend != false {
			t.Errorf("expected AutoExtend = false, got %v", params.cfg.AutoExtend)
		}
		if params.cfg.AutoExtendFactor != 2.0 {
			t.Errorf("expected AutoExtendFactor = 2.0, got %v", params.cfg.AutoExtendFactor)
		}
		if params.cfg.AutoExtendMax != 64*time.Minute {
			t.Errorf("expected AutoExtendMax = 64m, got %v", params.cfg.AutoExtendMax)
		}
	})

	t.Run("enable auto-extend with custom factor and max", func(t *testing.T) {
		args := []string{
			"-queue", "test-queue",
			"-auto-extend",
			"-auto-extend-factor", "3.0",
			"-auto-extend-max", "2h",
			"echo", "hello",
		}
		params, err := parseFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if params.cfg.AutoExtend != true {
			t.Errorf("expected AutoExtend = true, got %v", params.cfg.AutoExtend)
		}
		if params.cfg.AutoExtendFactor != 3.0 {
			t.Errorf("expected AutoExtendFactor = 3.0, got %v", params.cfg.AutoExtendFactor)
		}
		if params.cfg.AutoExtendMax != 2*time.Hour {
			t.Errorf("expected AutoExtendMax = 2h, got %v", params.cfg.AutoExtendMax)
		}
	})

	t.Run("auto-extend-factor below min bound", func(t *testing.T) {
		args := []string{
			"-queue", "test-queue",
			"-auto-extend",
			"-auto-extend-factor", "0.5",
			"echo", "hello",
		}
		_, err := parseFlags(args)
		if err == nil {
			t.Errorf("expected error when auto-extend-factor < 1.0, got nil")
		}
	})

	t.Run("auto-extend-factor above max bound", func(t *testing.T) {
		args := []string{
			"-queue", "test-queue",
			"-auto-extend",
			"-auto-extend-factor", "10.5",
			"echo", "hello",
		}
		_, err := parseFlags(args)
		if err == nil {
			t.Errorf("expected error when auto-extend-factor > 10.0, got nil")
		}
	})

	t.Run("auto-extend-max below min bound", func(t *testing.T) {
		args := []string{
			"-queue", "test-queue",
			"-auto-extend",
			"-auto-extend-max", "30s",
			"echo", "hello",
		}
		_, err := parseFlags(args)
		if err == nil {
			t.Errorf("expected error when auto-extend-max < 1m, got nil")
		}
	})

	t.Run("auto-extend-max above max bound", func(t *testing.T) {
		args := []string{
			"-queue", "test-queue",
			"-auto-extend",
			"-auto-extend-max", "5h",
			"echo", "hello",
		}
		_, err := parseFlags(args)
		if err == nil {
			t.Errorf("expected error when auto-extend-max > 4h, got nil")
		}
	})
}
