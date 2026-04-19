package actions

import (
	"os"
	"testing"
)

func TestSample_ParseTopStacks(t *testing.T) {
	data, err := os.ReadFile("testdata/sample_chrome.txt")
	if err != nil {
		t.Fatal(err)
	}
	stacks := parseSampleOutput(string(data))
	if len(stacks) == 0 {
		t.Fatal("expected at least one stack")
	}
	if stacks[0].Percent < 90 {
		t.Errorf("expected top stack >=90%%, got %.1f", stacks[0].Percent)
	}
	if stacks[0].Top == "" {
		t.Error("expected non-empty Top function")
	}
}
