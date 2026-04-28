package filter

import "testing"

func TestPresetManager(t *testing.T) {
	presets := map[string]string{
		"urgent": "label:urgent status:open",
		"mine":   "author:alice",
	}
	pm := NewPresetManager(presets)

	t.Run("List returns all presets", func(t *testing.T) {
		list := pm.List()
		if len(list) != 2 {
			t.Fatalf("got %d presets, want 2", len(list))
		}
		if list["urgent"] != "label:urgent status:open" {
			t.Errorf("unexpected value for urgent: %q", list["urgent"])
		}
	})

	t.Run("Get existing preset", func(t *testing.T) {
		fs, ok := pm.Get("urgent")
		if !ok {
			t.Fatal("expected ok")
		}
		if len(fs.Filters) != 2 {
			t.Errorf("got %d filters, want 2", len(fs.Filters))
		}
	})

	t.Run("Get missing preset", func(t *testing.T) {
		_, ok := pm.Get("nonexistent")
		if ok {
			t.Error("expected not ok")
		}
	})

	t.Run("nil presets", func(t *testing.T) {
		pm := NewPresetManager(nil)
		if pm.List() == nil {
			t.Error("List should not be nil even with nil input")
		}
		_, ok := pm.Get("anything")
		if ok {
			t.Error("expected not ok from nil presets")
		}
	})
}
