package main

import "testing"

func TestMemoryScopeToggleOverridesAggregateSetting(t *testing.T) {
	modules := []string{
		statusRightModuleWindowMemory,
		statusRightModuleSessionMemory,
		statusRightModuleTotalMemory,
	}

	for _, module := range modules {
		t.Run(module, func(t *testing.T) {
			cfg := statusRightConfig{MemoryTotals: boolPtr(false)}

			cfg.setModuleEnabled(module, true)
			if !cfg.moduleEnabled(module) {
				t.Fatalf("expected %s to override disabled memory totals", module)
			}

			cfg.setModuleEnabled(module, false)
			if cfg.moduleEnabled(module) {
				t.Fatalf("expected %s to inherit disabled memory totals", module)
			}
		})
	}
}

func TestMemoryScopeToggleOverridesEnabledAggregateSetting(t *testing.T) {
	modules := []string{
		statusRightModuleWindowMemory,
		statusRightModuleSessionMemory,
		statusRightModuleTotalMemory,
	}

	for _, module := range modules {
		t.Run(module, func(t *testing.T) {
			cfg := statusRightConfig{MemoryTotals: boolPtr(true)}

			cfg.setModuleEnabled(module, false)
			if cfg.moduleEnabled(module) {
				t.Fatalf("expected %s to override enabled memory totals", module)
			}

			cfg.setModuleEnabled(module, true)
			if !cfg.moduleEnabled(module) {
				t.Fatalf("expected %s to inherit enabled memory totals", module)
			}
		})
	}
}
