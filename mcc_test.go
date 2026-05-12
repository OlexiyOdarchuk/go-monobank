package monobank

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMCC_Category(t *testing.T) {
	tests := map[MCC]MCCCategory{
		4829: CategoryTransfer,    // money transfer (mono jars, p2p)
		5411: CategoryGroceries,   // grocery stores / supermarkets
		5541: CategoryFuel,        // service stations
		5812: CategoryRestaurants, // eating places
		4812: CategoryTelecom,     // telecom equipment
		4111: CategoryTransport,   // local commuter passenger transport
		8011: CategoryHealth,      // doctors
		8211: CategoryEducation,   // elementary/secondary schools
		8398: CategoryCharity,     // charitable & social service orgs
		9311: CategoryGovernment,  // tax payments
		1234: CategoryAgriculture, // < 1500
		0:    CategoryUnknown,
	}
	for code, want := range tests {
		assert.Equalf(t, want, code.Category(), "MCC %d", int(code))
	}
}

func TestTransaction_MCCCode(t *testing.T) {
	tx := Transaction{MCC: 5411, OriginalMCC: 4829}
	assert.Equal(t, MCC(5411), tx.MCCCode())
	assert.Equal(t, CategoryGroceries, tx.MCCCode().Category())
	assert.Equal(t, MCC(4829), tx.OriginalMCCCode())
	assert.Equal(t, CategoryTransfer, tx.OriginalMCCCode().Category())
}
