package main

import (
	"encoding/json"
	"testing"
)

func TestAPIConfigUnmarshalDefaultsUseStreamToTrue(t *testing.T) {
	var cfg APIConfig
	if err := json.Unmarshal([]byte(`{"base_url":"http://example.com","model":"m"}`), &cfg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !cfg.UseStream {
		t.Fatal("UseStream = false, want true when field omitted")
	}
}

func TestAPIConfigUnmarshalHonorsExplicitUseStreamFalse(t *testing.T) {
	var cfg APIConfig
	if err := json.Unmarshal([]byte(`{"base_url":"http://example.com","model":"m","use_stream":false}`), &cfg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if cfg.UseStream {
		t.Fatal("UseStream = true, want false when field explicitly set")
	}
}