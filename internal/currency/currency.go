// Package currency renders ISO 4217 currency codes the way people actually
// write them. Nobody writes "USD3117.48" on a bill; they write "$3,117.48".
package currency

import (
	"strings"
	"unicode"
)

// Style says how one currency is written: which glyph, how many decimal
// places it conventionally carries, and whether the glyph trails the number.
type Style struct {
	Symbol   string
	Decimals int  // minor units; the zero value means the usual two
	Suffix   bool // true when the symbol follows the amount, as in "120 kr"
}

// none marks a currency with no minor unit, such as the yen. The zero value
// of Style.Decimals already means "the usual two", so currencies that really
// do round to whole units need a value of their own.
const none = -1

// styles maps ISO 4217 codes to their display style. Many currencies share a
// glyph on purpose: a Canadian or Australian dollar reads as "$" to the
// person looking at the bill, and the code is redundant on a personal
// dashboard that is not mixing currencies side by side. Codes absent here
// fall back to the code itself as a prefix, which is always unambiguous.
var styles = map[string]Style{
	// Dollar-denominated
	"USD": {Symbol: "$"},
	"CAD": {Symbol: "$"},
	"AUD": {Symbol: "$"},
	"NZD": {Symbol: "$"},
	"SGD": {Symbol: "$"},
	"HKD": {Symbol: "$"},
	"TWD": {Symbol: "$"},
	"MXN": {Symbol: "$"},
	"ARS": {Symbol: "$"},
	"CLP": {Symbol: "$", Decimals: none},
	"COP": {Symbol: "$"},
	"UYU": {Symbol: "$"},
	"BRL": {Symbol: "R$"},

	// Europe
	"EUR": {Symbol: "€"},
	"GBP": {Symbol: "£"},
	"CHF": {Symbol: "CHF "},
	"SEK": {Symbol: " kr", Suffix: true},
	"NOK": {Symbol: " kr", Suffix: true},
	"DKK": {Symbol: " kr", Suffix: true},
	"ISK": {Symbol: " kr", Suffix: true, Decimals: none},
	"PLN": {Symbol: " zł", Suffix: true},
	"CZK": {Symbol: " Kč", Suffix: true},
	"HUF": {Symbol: " Ft", Suffix: true, Decimals: none},
	"RON": {Symbol: " lei", Suffix: true},
	"BGN": {Symbol: " лв", Suffix: true},
	"UAH": {Symbol: "₴"},
	"RUB": {Symbol: "₽"},
	"TRY": {Symbol: "₺"},

	// Asia-Pacific
	"JPY": {Symbol: "¥", Decimals: none},
	"CNY": {Symbol: "¥"},
	"KRW": {Symbol: "₩", Decimals: none},
	"INR": {Symbol: "₹"},
	"PKR": {Symbol: "₨"},
	"LKR": {Symbol: "₨"},
	"NPR": {Symbol: "₨"},
	"THB": {Symbol: "฿"},
	"VND": {Symbol: "₫", Decimals: none},
	"PHP": {Symbol: "₱"},
	"IDR": {Symbol: "Rp", Decimals: none},
	"MYR": {Symbol: "RM"},
	"BDT": {Symbol: "৳"},
	"KHR": {Symbol: "៛"},
	"MNT": {Symbol: "₮"},

	// Middle East & Africa
	"ILS": {Symbol: "₪"},
	"SAR": {Symbol: "﷼"},
	"AED": {Symbol: "د.إ "},
	"QAR": {Symbol: "﷼"},
	"IRR": {Symbol: "﷼"},
	"EGP": {Symbol: "£"},
	"NGN": {Symbol: "₦"},
	"GHS": {Symbol: "₵"},
	"KES": {Symbol: "KSh "},
	"ZAR": {Symbol: "R"},
	"MAD": {Symbol: "DH "},

	// Precious metals & crypto occasionally seen in Paperless
	"XAU": {Symbol: "XAU ", Decimals: 4},
	"BTC": {Symbol: "₿", Decimals: 8},
}

// StyleFor returns the display style for a currency code. An unknown or
// empty code yields a style that prints the code itself, so an unfamiliar
// currency is never silently mislabelled as dollars.
func StyleFor(code string) Style {
	code = strings.ToUpper(strings.TrimSpace(code))
	s, ok := styles[code]
	if !ok && code != "" {
		s = Style{Symbol: code + " "}
	}
	switch {
	case s.Decimals == none:
		s.Decimals = 0
	case s.Decimals == 0:
		s.Decimals = 2
	}
	return s
}

// Split separates a Paperless monetary value into its currency code and its
// numeric part. Paperless writes "USD174.87" when a document names its own
// currency and a bare "174.87" when it inherits the field's default, so the
// code is optional and an empty first return means "not stated here".
func Split(value string) (code, amount string) {
	value = strings.TrimSpace(value)
	i := 0
	for i < len(value) && i < 3 && value[i] >= 'A' && value[i] <= 'Z' {
		i++
	}
	if i == 3 && len(value) > 3 && (unicode.IsDigit(rune(value[3])) || value[3] == '-' || value[3] == '.') {
		return value[:3], value[3:]
	}
	return "", value
}

// Format renders a numeric string in the given currency, grouping thousands
// and fixing the decimal places to the currency's convention. A value that is
// not a plain decimal number is passed through untouched rather than mangled.
func Format(code, amount string) string {
	st := StyleFor(code)
	n, ok := normalize(amount, st.Decimals)
	if !ok {
		return strings.TrimSpace(strings.TrimSpace(st.Symbol) + " " + amount)
	}
	if st.Suffix {
		return n + st.Symbol
	}
	if strings.HasPrefix(n, "-") {
		return "-" + st.Symbol + n[1:]
	}
	return st.Symbol + n
}

// normalize groups the integer part in threes and pads or trims the fraction
// to decimals places. It works on the string rather than a float so that a
// value Paperless stored exactly is displayed exactly.
func normalize(amount string, decimals int) (string, bool) {
	s := strings.TrimSpace(amount)
	if s == "" {
		return "", false
	}
	neg := false
	if s[0] == '-' || s[0] == '+' {
		neg = s[0] == '-'
		s = s[1:]
	}
	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if !allDigits(intPart) || !allDigits(frac) {
		return "", false
	}

	if len(frac) > decimals {
		frac = frac[:decimals]
	}
	for len(frac) < decimals {
		frac += "0"
	}

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if frac != "" {
		b.WriteByte('.')
		b.WriteString(frac)
	}
	return b.String(), true
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
