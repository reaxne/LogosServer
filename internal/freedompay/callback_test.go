package freedompay

import "testing"

func TestParseAmountCents(t *testing.T) {
	tests := map[string]int64{
		"10":    1000,
		"10.5":  1050,
		"10.05": 1005,
		".99":   99,
		"0.01":  1,
	}

	for input, want := range tests {
		got, err := parseAmountCents(input)
		if err != nil {
			t.Fatalf("parseAmountCents(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseAmountCents(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestParseAmountCentsRejectsNegative(t *testing.T) {
	if _, err := parseAmountCents("-0.50"); err == nil {
		t.Fatal("expected negative amount to fail")
	}
}

func TestParseAmountCentsRejectsMoreThanTwoDecimals(t *testing.T) {
	if _, err := parseAmountCents("123.456"); err == nil {
		t.Fatal("expected over-precise amount to fail")
	}
}
