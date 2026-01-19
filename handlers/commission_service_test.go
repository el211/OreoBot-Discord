package handlers

import (
	"testing"

	"discord-bot/config"
)

func TestFindServiceByName(t *testing.T) {
	cs := &CommissionService{
		cfg: &config.Config{
			Commissions: config.CommissionsConfig{
				Services: []config.CommissionService{
					{Name: "Logo Design", StartingPrice: "Starting at $50"},
					{Name: "Banner", StartingPrice: "Starting at $25"},
				},
			},
		},
	}

	svc, ok := cs.FindServiceByName("Logo Design")
	if !ok {
		t.Fatal("expected to find Logo Design")
	}
	if svc.StartingPrice != "Starting at $50" {
		t.Errorf("expected starting price 'Starting at $50', got %q", svc.StartingPrice)
	}

	_, ok = cs.FindServiceByName("Nonexistent")
	if ok {
		t.Error("expected Nonexistent to not be found")
	}
}

func TestFindServiceByNameCaseSensitive(t *testing.T) {
	cs := &CommissionService{
		cfg: &config.Config{
			Commissions: config.CommissionsConfig{
				Services: []config.CommissionService{
					{Name: "logo", StartingPrice: "Starting at $10"},
				},
			},
		},
	}
	_, ok := cs.FindServiceByName("Logo")
	if ok {
		t.Error("service lookup should be case-sensitive")
	}
	_, ok = cs.FindServiceByName("logo")
	if !ok {
		t.Error("exact match should be found")
	}
}
