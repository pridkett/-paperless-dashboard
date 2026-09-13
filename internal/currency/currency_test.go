package currency

import "testing"

func TestSplit(t *testing.T) {
	cases := []struct {
		in         string
		code, want string
	}{
		{"USD174.87", "USD", "174.87"},
		{"3117.48", "", "3117.48"},
		{"GBP-12.50", "GBP", "-12.50"},
		{"JPY1200", "JPY", "1200"},
		{"EUR.50", "EUR", ".50"},
		{"", "", ""},
		{"USD", "", "USD"}, // a code with no amount is not a split
		{"n/a", "", "n/a"}, // lowercase never reads as a code
		{"USDX1", "", "USDX1"},
	}
	for _, c := range cases {
		code, amount := Split(c.in)
		if code != c.code || amount != c.want {
			t.Errorf("Split(%q) = (%q, %q), want (%q, %q)", c.in, code, amount, c.code, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	cases := []struct{ code, amount, want string }{
		{"USD", "3117.48", "$3,117.48"},
		{"USD", "87.11", "$87.11"},
		{"USD", "1234567.5", "$1,234,567.50"},
		{"CAD", "50", "$50.00"},
		{"AUD", "50", "$50.00"},
		{"GBP", "12.5", "£12.50"},
		{"EUR", "1000", "€1,000.00"},
		{"JPY", "1200", "¥1,200"},
		{"JPY", "1200.60", "¥1,200"},
		{"SEK", "1200", "1,200.00 kr"},
		{"USD", "-45.20", "-$45.20"},
		{"SEK", "-45.20", "-45.20 kr"},
		{"", "45.20", "45.20"},
		{"ZZZ", "45.2", "ZZZ 45.20"},
		{"USD", "n/a", "$ n/a"},
	}
	for _, c := range cases {
		if got := Format(c.code, c.amount); got != c.want {
			t.Errorf("Format(%q, %q) = %q, want %q", c.code, c.amount, got, c.want)
		}
	}
}

func TestStyleForUnknownCodeKeepsCode(t *testing.T) {
	s := StyleFor("xyz")
	if s.Symbol != "XYZ " {
		t.Errorf("Symbol = %q, want %q", s.Symbol, "XYZ ")
	}
	if s.Decimals != 2 {
		t.Errorf("Decimals = %d, want 2", s.Decimals)
	}
}
