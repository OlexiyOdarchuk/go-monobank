package monobank

import "strconv"

// CurrencyCode is an ISO 4217 numeric currency code. The bank returns it as
// a plain int in payloads (Account.CurrencyCode, Transaction.CurrencyCode,
// Jar.CurrencyCode, Currency.CurrencyCodeA/B); callers can convert with
// monobank.CurrencyCode(t.CurrencyCode) for type-safe comparisons.
type CurrencyCode int

// Currency codes seen in monobank payloads. Not exhaustive — extend as needed.
const (
	UAH CurrencyCode = 980 // Ukrainian hryvnia (account default)
	USD CurrencyCode = 840 // US dollar
	EUR CurrencyCode = 978 // Euro
	GBP CurrencyCode = 826 // Pound sterling
	PLN CurrencyCode = 985 // Polish złoty
	CHF CurrencyCode = 756 // Swiss franc
	JPY CurrencyCode = 392 // Japanese yen
	CZK CurrencyCode = 203 // Czech koruna
	CAD CurrencyCode = 124 // Canadian dollar
	AUD CurrencyCode = 36  // Australian dollar
	CNY CurrencyCode = 156 // Chinese yuan
)

// alpha3 maps numeric ISO 4217 codes to their alphabetic counterparts for the
// currencies declared above.
var alpha3 = map[CurrencyCode]string{
	UAH: "UAH",
	USD: "USD",
	EUR: "EUR",
	GBP: "GBP",
	PLN: "PLN",
	CHF: "CHF",
	JPY: "JPY",
	CZK: "CZK",
	CAD: "CAD",
	AUD: "AUD",
	CNY: "CNY",
}

// String returns the ISO 4217 alphabetic code (e.g. "UAH") if known, otherwise
// the numeric code as a decimal string.
func (c CurrencyCode) String() string {
	if s, ok := alpha3[c]; ok {
		return s
	}
	return strconv.Itoa(int(c))
}
