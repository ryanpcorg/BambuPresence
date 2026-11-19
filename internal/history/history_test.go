package history

import (
	"path/filepath"
	"testing"

	"bambupresence/internal/bambu"
)

func TestHistoryLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	mgr := NewManager(path, nil)

	mgr.Observe(bambu.PrinterState{
		GcodeState:  "RUNNING",
		SubtaskName: "part_v2.3mf",
		Percent:     15,
		Layer:       20,
		TotalLayers: 200,
		Model:       bambu.PrinterModel{Name: "Bambu Lab P1S"},
	})
	mgr.Observe(bambu.PrinterState{
		GcodeState:  "RUNNING",
		SubtaskName: "part_v2.3mf",
		Percent:     98,
		Layer:       199,
		TotalLayers: 200,
		Model:       bambu.PrinterModel{Name: "Bambu Lab P1S"},
	})
	mgr.Observe(bambu.PrinterState{
		GcodeState:  "FINISH",
		SubtaskName: "part_v2.3mf",
		Percent:     100,
		Layer:       200,
		TotalLayers: 200,
		Model:       bambu.PrinterModel{Name: "Bambu Lab P1S"},
	})

	entries := mgr.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(entries))
	}
	if entries[0].Status != "Completed" || entries[0].FileName != "part_v2.3mf" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}

	stats := mgr.Stats()
	if stats.TotalPrints != 1 || stats.CompletedPrints != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
