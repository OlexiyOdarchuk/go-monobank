package monobank

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrencyCode_String(t *testing.T) {
	tests := map[CurrencyCode]string{
		UAH:                "UAH",
		USD:                "USD",
		EUR:                "EUR",
		GBP:                "GBP",
		PLN:                "PLN",
		CurrencyCode(7777): "7777", // unknown — falls back to decimal
	}
	for code, want := range tests {
		assert.Equalf(t, want, code.String(), "String() for %d", int(code))
	}
}
