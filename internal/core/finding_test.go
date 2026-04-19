package core

import "testing"

func TestSeverity_String(t *testing.T) {
	if SeverityHigh.String() != "high" {
		t.Errorf("expected 'high'")
	}
	if SeverityCritical.String() != "critical" {
		t.Errorf("expected 'critical'")
	}
}
