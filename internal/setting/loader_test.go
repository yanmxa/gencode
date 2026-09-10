package setting

import "testing"

func TestLoaderLoad(t *testing.T) {
	settings, err := NewLoader().Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if settings == nil {
		t.Fatal("Load() returned nil settings")
	}
}
