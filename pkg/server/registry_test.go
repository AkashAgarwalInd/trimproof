package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRegistryOutputPriceRatio(t *testing.T) {
	dir := t.TempDir()
	load := func(body string) (*Registry, error) {
		path := filepath.Join(dir, "p.json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return LoadRegistry(path)
	}
	reg, err := load(`{"routes":[{"route_id":"a","codec":"toon"},{"route_id":"b","codec":"toon","output_price_ratio":5}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := reg.Lookup("t", "a").Policy.OutputPriceRatio; got != 4 {
		t.Fatalf("default output_price_ratio %v", got)
	}
	if got := reg.Lookup("t", "b").Policy.OutputPriceRatio; got != 5 {
		t.Fatalf("output_price_ratio %v", got)
	}
	if _, err := load(`{"routes":[{"route_id":"c","output_price_ratio":-1}]}`); err == nil {
		t.Fatal("negative output_price_ratio accepted")
	}
	if _, err := LoadRegistry("../../examples/policies.json"); err != nil {
		t.Fatalf("example policies: %v", err)
	}
}
