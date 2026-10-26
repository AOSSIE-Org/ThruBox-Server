package model

import "testing"

func TestContentID(t *testing.T) {
	base := ContentID("0xto", "0xfrom", "payload")

	if len(base) != 64 {
		t.Fatalf("ContentID length = %d, want 64 hex characters", len(base))
	}
	if again := ContentID("0xto", "0xfrom", "payload"); again != base {
		t.Errorf("ContentID is not deterministic: %q then %q", base, again)
	}

	differ := map[string]string{
		"different recipient": ContentID("0xother", "0xfrom", "payload"),
		"different sender":    ContentID("0xto", "0xother", "payload"),
		"different payload":   ContentID("0xto", "0xfrom", "other"),
		// Without length prefixes these two would hash the same bytes.
		"bytes moved between fields": ContentID("0xt", "o0xfrom", "payload"),
	}
	for name, id := range differ {
		if id == base {
			t.Errorf("%s: got the same ID %q", name, id)
		}
	}
}
