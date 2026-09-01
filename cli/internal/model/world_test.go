package model

import (
	"testing"
	"time"

	"llamalens-cli/internal/cfg"
)

func TestBuildSnapshotMMProj(t *testing.T) {
	w := NewWorld(cfg.Default(), NewEventDetector(50), NewDiffEngine(), NewRingBuffer(3600), NewRingBuffer(1800))
	w.SetHost(HostMetrics{
		Process: &ProcInfo{Found: true, PID: 1, Flags: map[string]string{"mmproj": "/tmp/fake-mmproj.bin"}},
	})
	snap := w.buildSnapshot(float64(time.Now().Unix()), 0, 0, "api")
	if snap.Llama.Model.MMProjPath != "/tmp/fake-mmproj.bin" {
		t.Fatalf("MMProjPath 未设置: %+v", snap.Llama.Model)
	}
}
