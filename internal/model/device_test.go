package model

import (
	"testing"

	"github.com/wimmme/shellylanman/internal/parse"
)

func TestApplyReadingsUpdate(t *testing.T) { // DECISIONS P19-1
	d := Device{Gen: "2"}
	d.ApplyReadings(parse.Readings{UpdateAvailable: true, UpdateVersion: "1.4.4"})
	if !d.UpdateAvailable || d.UpdateVersion != "1.4.4" {
		t.Fatalf("update not copied: %+v", d)
	}
	d.ApplyReadings(parse.Readings{}) // installed: the arrow goes away
	if d.UpdateAvailable || d.UpdateVersion != "" {
		t.Fatalf("update not cleared: %+v", d)
	}
}
