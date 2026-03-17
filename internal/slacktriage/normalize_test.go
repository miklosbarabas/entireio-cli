package slacktriage

import "testing"

func TestNormalizeTrigger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "preserves_exact_trigger",
			in:   "triage e2e",
			want: "triage e2e",
		},
		{
			name: "lowercases_and_trims",
			in:   "  Triage E2E  ",
			want: "triage e2e",
		},
		{
			name: "collapses_internal_whitespace",
			in:   "triage   e2e",
			want: "triage e2e",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeTrigger(tt.in); got != tt.want {
				t.Fatalf("NormalizeTrigger(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsTriageTrigger(t *testing.T) {
	t.Parallel()

	if !IsTriageTrigger("  Triage   E2E  ") {
		t.Fatal("expected normalized trigger to match")
	}

	for _, in := range []string{"triage", "triage e2e now", "triage-e2e"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if IsTriageTrigger(in) {
				t.Fatalf("IsTriageTrigger(%q) = true, want false", in)
			}
		})
	}
}
