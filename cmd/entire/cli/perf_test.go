package cli

import (
	"testing"
	"time"
)

func TestParsePerfEntry(t *testing.T) {
	t.Parallel()

	t.Run("valid perf entry", func(t *testing.T) {
		t.Parallel()

		line := `{"time":"2026-01-15T10:30:00.000Z","level":"DEBUG","msg":"perf","component":"perf","op":"post-commit","duration_ms":150,"error":true,"steps.load_session_ms":50,"steps.save_checkpoint_ms":80,"steps.save_checkpoint_err":true}`

		entry := parsePerfEntry(line)
		if entry == nil {
			t.Fatal("parsePerfEntry returned nil for valid perf entry")
		}

		if entry.Op != "post-commit" {
			t.Errorf("Op = %q, want %q", entry.Op, "post-commit")
		}
		if entry.DurationMs != 150 {
			t.Errorf("DurationMs = %d, want %d", entry.DurationMs, 150)
		}
		if !entry.Error {
			t.Error("Error = false, want true")
		}

		expectedTime, err := time.Parse(time.RFC3339, "2026-01-15T10:30:00.000Z")
		if err != nil {
			t.Fatalf("failed to parse expected time: %v", err)
		}
		if !entry.Time.Equal(expectedTime) {
			t.Errorf("Time = %v, want %v", entry.Time, expectedTime)
		}

		if len(entry.Steps) != 2 {
			t.Fatalf("len(Steps) = %d, want 2", len(entry.Steps))
		}

		// Steps are sorted alphabetically by name
		if entry.Steps[0].Name != "load_session" {
			t.Errorf("Steps[0].Name = %q, want %q", entry.Steps[0].Name, "load_session")
		}
		if entry.Steps[0].DurationMs != 50 {
			t.Errorf("Steps[0].DurationMs = %d, want %d", entry.Steps[0].DurationMs, 50)
		}
		if entry.Steps[0].Error {
			t.Error("Steps[0].Error = true, want false")
		}

		if entry.Steps[1].Name != "save_checkpoint" {
			t.Errorf("Steps[1].Name = %q, want %q", entry.Steps[1].Name, "save_checkpoint")
		}
		if entry.Steps[1].DurationMs != 80 {
			t.Errorf("Steps[1].DurationMs = %d, want %d", entry.Steps[1].DurationMs, 80)
		}
		if !entry.Steps[1].Error {
			t.Error("Steps[1].Error = false, want true")
		}
	})

	t.Run("non-perf entry returns nil", func(t *testing.T) {
		t.Parallel()

		line := `{"time":"2026-01-15T10:30:00.000Z","level":"INFO","msg":"hook invoked","component":"lifecycle","hook":"post-commit"}`

		entry := parsePerfEntry(line)
		if entry != nil {
			t.Errorf("parsePerfEntry returned %+v for non-perf entry, want nil", entry)
		}
	})

	t.Run("invalid JSON returns nil", func(t *testing.T) {
		t.Parallel()

		entry := parsePerfEntry("this is not json at all{{{")
		if entry != nil {
			t.Errorf("parsePerfEntry returned %+v for invalid JSON, want nil", entry)
		}
	})
}
