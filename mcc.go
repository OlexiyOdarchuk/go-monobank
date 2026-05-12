package monobank

// MCC is an ISO 18245 Merchant Category Code. Mono populates it on every
// statement transaction (Transaction.MCC, Transaction.OriginalMCC); use
// this type's helpers to bucket spending by category without memorising
// four-digit codes.
type MCC int

// MCCCategory groups merchant categories at the level most apps want for
// reporting (groceries vs restaurants, fuel vs transport, etc.).
type MCCCategory string

// Known categories. Not exhaustive — extend as needed.
const (
	CategoryUnknown      MCCCategory = "Unknown"
	CategoryAgriculture  MCCCategory = "Agriculture"
	CategoryContracted   MCCCategory = "ContractedServices"
	CategoryTransport    MCCCategory = "Transport"
	CategoryFuel         MCCCategory = "Fuel"
	CategoryUtilities    MCCCategory = "Utilities"
	CategoryRetail       MCCCategory = "Retail"
	CategoryGroceries    MCCCategory = "Groceries"
	CategoryClothing     MCCCategory = "Clothing"
	CategoryEntertain    MCCCategory = "Entertainment"
	CategoryRestaurants  MCCCategory = "Restaurants"
	CategoryHotels       MCCCategory = "Hotels"
	CategoryHealth       MCCCategory = "Health"
	CategoryEducation    MCCCategory = "Education"
	CategoryProfessional MCCCategory = "ProfessionalServices"
	CategoryFinancial    MCCCategory = "Financial"
	CategoryTransfer     MCCCategory = "MoneyTransfer"
	CategoryGovernment   MCCCategory = "Government"
	CategoryTelecom      MCCCategory = "Telecom"
	CategoryCharity      MCCCategory = "Charity"
)

// Category returns a high-level bucket for the MCC based on the ISO 18245
// range tables. Unknown codes return [CategoryUnknown].
//
// Specific codes are matched before broader ranges (first-match wins).
func (c MCC) Category() MCCCategory {
	switch {
	// --- specific codes that override their containing range ---
	case c == 4829:
		return CategoryTransfer
	case c == 4812, c == 4813, c == 4814, c == 4816, c == 4821, c == 4899:
		return CategoryTelecom
	case c == 5411, c == 5422, c == 5441, c == 5451, c == 5462, c == 5499:
		return CategoryGroceries
	case c == 5541, c == 5542, c == 5552, c == 5983:
		return CategoryFuel
	case c >= 5811 && c <= 5814:
		return CategoryRestaurants
	case c == 8398:
		return CategoryCharity

	// --- broad ranges ---
	case c >= 1 && c <= 1499:
		return CategoryAgriculture
	case c >= 1500 && c <= 2999:
		return CategoryContracted
	case c >= 3000 && c <= 3999:
		return CategoryTransport // airlines + car-rental
	case c >= 4000 && c <= 4799:
		return CategoryTransport
	case c >= 4900 && c <= 4999:
		return CategoryUtilities
	case c >= 5000 && c <= 5599:
		return CategoryRetail
	case c >= 5600 && c <= 5699:
		return CategoryClothing
	case c >= 5700 && c <= 5999:
		return CategoryRetail
	case c >= 6000 && c <= 6999:
		return CategoryFinancial
	case c >= 7000 && c <= 7299:
		return CategoryHotels
	case c >= 7800 && c <= 7999:
		return CategoryEntertain
	case c >= 8000 && c <= 8099:
		return CategoryHealth
	case c >= 8200 && c <= 8299:
		return CategoryEducation
	case c >= 8300 && c <= 8999:
		return CategoryProfessional
	case c >= 9000 && c <= 9999:
		return CategoryGovernment
	}
	return CategoryUnknown
}

// MCC returns the typed MCC for a transaction's numeric code.
func (t Transaction) MCCCode() MCC { return MCC(t.MCC) }

// OriginalMCCCode is the pre-categorisation MCC from the acquirer; mono
// sometimes remaps MCC (e.g. for cashback), keeping the original here.
func (t Transaction) OriginalMCCCode() MCC { return MCC(t.OriginalMCC) }
