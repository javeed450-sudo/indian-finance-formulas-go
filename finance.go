// Package indianfinance implements zero-dependency finance formulas for India —
// loans, investments, GST, gratuity and income tax.
//
// Every function here is extracted from the calculators running at
// https://emicalcs.com and is covered by the test suite in finance_test.go,
// which is a direct port of the JavaScript suite the same formulas ship with.
//
// Statutory figures — tax slabs, gratuity ceilings, GST rates — change by
// notification. Where a function depends on one, the value is a documented
// OPTION with a default rather than a hidden constant, so you can update it
// yourself the day it moves without waiting for a release.
//
// # A note on types
//
// The JavaScript original takes months and years as numbers and rounds them.
// This port takes them as int where the underlying loop is integer-stepped,
// which removes the NaN and Infinity cases the JS version has to guard against
// at runtime. The tenure ceiling below is kept regardless.
//
// MIT licensed.
package indianfinance

import (
	"fmt"
	"math"
)

// Every schedule function below steps one period at a time, so the loop is
// bounded only by the tenure it is handed. Left unchecked, a bad tenure is an
// out-of-memory crash in the CALLER's process — not a slow answer — and there
// is no UI here to clamp the input first. 1200 months / 100 years is far beyond
// anything real (the longest home loan on offer runs 30-40 years), so the
// ceiling only ever fires on input that was never going to mean anything.
const (
	MaxMonths = 1200
	MaxYears  = MaxMonths / 12
)

// ErrTenure is returned when a tenure exceeds the ceiling. Use errors.Is to
// match it; the wrapped message carries the offending value.
type ErrTenure struct {
	Unit  string
	Got   int
	Limit int
}

func (e *ErrTenure) Error() string {
	return fmt.Sprintf("%s must be at most %d, got %d", e.Unit, e.Limit, e.Got)
}

func checkMonths(months int) error {
	if months > MaxMonths {
		return &ErrTenure{Unit: "months", Got: months, Limit: MaxMonths}
	}
	return nil
}

func checkYears(years int) error {
	if years > MaxYears {
		return &ErrTenure{Unit: "years", Got: years, Limit: MaxYears}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Loans
// ---------------------------------------------------------------------------

// EMI returns the reducing-balance monthly instalment.
// annualRate is a percentage, e.g. 8.5 for 8.5% p.a.
func EMI(principal, annualRate float64, months int) float64 {
	p := math.Max(0, principal)
	if p <= 0 || months <= 0 {
		return 0
	}
	r := annualRate / 12 / 100
	if r == 0 {
		return p / float64(months)
	}
	return (p * r * math.Pow(1+r, float64(months))) / (math.Pow(1+r, float64(months)) - 1)
}

// TotalInterest returns the total interest paid over the full tenure.
func TotalInterest(principal, annualRate float64, months int) float64 {
	return math.Max(0, EMI(principal, annualRate, months)*float64(months)-principal)
}

// AmortRow is one month of an amortisation schedule.
type AmortRow struct {
	Month     int
	Opening   float64
	Interest  float64
	Principal float64
	Closing   float64
}

// Amortisation returns the month-by-month schedule.
// It returns an *ErrTenure if months exceeds MaxMonths.
func Amortisation(principal, annualRate float64, months int) ([]AmortRow, error) {
	if err := checkMonths(months); err != nil {
		return nil, err
	}
	r := annualRate / 12 / 100
	e := EMI(principal, annualRate, months)
	bal := principal
	out := make([]AmortRow, 0, months)
	for m := 1; m <= months && bal > 0.005; m++ {
		i := bal * r
		p := e - i
		if p > bal {
			p = bal
		}
		out = append(out, AmortRow{Month: m, Opening: bal, Interest: i, Principal: p, Closing: bal - p})
		bal -= p
	}
	return out, nil
}

// PrepaymentInput describes a prepayment scenario. The EMI is held fixed, so
// prepaying shortens the tenure rather than reducing the instalment.
type PrepaymentInput struct {
	Principal      float64
	AnnualRate     float64
	Months         int // original tenure
	MonthlyExtra   float64
	LumpSum        float64
	LumpSumAtMonth int // 0 = before the first instalment
}

// PrepaymentResult is what prepaying achieves.
type PrepaymentResult struct {
	InterestSaved float64
	MonthsSaved   int
	NewMonths     int
	NewInterest   float64
}

// Prepayment computes the effect of prepaying with the EMI held fixed.
// It returns an *ErrTenure if Months exceeds MaxMonths.
func Prepayment(in PrepaymentInput) (PrepaymentResult, error) {
	if err := checkMonths(in.Months); err != nil {
		return PrepaymentResult{}, err
	}
	n := in.Months
	r := in.AnnualRate / 12 / 100
	e := EMI(in.Principal, in.AnnualRate, n)
	baseInterest := e*float64(n) - in.Principal

	bal := in.Principal
	if in.LumpSumAtMonth <= 0 && in.LumpSum > 0 {
		bal = math.Max(0, bal-in.LumpSum)
	}
	var newInterest float64
	newMonths := 0
	for m := 1; m <= n && bal > 0.005; m++ {
		i := bal * r
		p := e - i + in.MonthlyExtra
		if m == in.LumpSumAtMonth && in.LumpSum > 0 {
			p += in.LumpSum
		}
		if p > bal {
			p = bal
		}
		bal -= p
		newInterest += i
		newMonths = m
	}
	return PrepaymentResult{
		InterestSaved: math.Max(0, baseInterest-newInterest),
		MonthsSaved:   n - newMonths,
		NewMonths:     newMonths,
		NewInterest:   newInterest,
	}, nil
}

// ---------------------------------------------------------------------------
// Investments
// ---------------------------------------------------------------------------

// SIPFutureValue returns the future value of a monthly SIP, with the
// contribution made at the start of each month.
func SIPFutureValue(monthly, annualRate float64, months int) float64 {
	if months <= 0 {
		return 0
	}
	r := annualRate / 12 / 100
	if r == 0 {
		return monthly * float64(months)
	}
	return monthly * ((math.Pow(1+r, float64(months)) - 1) / r) * (1 + r)
}

// StepUpResult is the outcome of a step-up SIP.
type StepUpResult struct {
	FutureValue float64
	Invested    float64
	Gain        float64
}

// StepUpSIP models a SIP whose contribution rises by stepUpPct every 12 months.
//
// Worth knowing: a step-up SIP does NOT beat a flat SIP of the same total
// outlay. It wins in headline terms only because more money goes in. Hold the
// money constant and the flat schedule wins, because its rupees compound for
// longer. See https://emicalcs.com/step-up-sip-calculator/
//
// It returns an *ErrTenure if years exceeds MaxYears.
func StepUpSIP(monthly, annualRate float64, years int, stepUpPct float64) (StepUpResult, error) {
	if err := checkYears(years); err != nil {
		return StepUpResult{}, err
	}
	r := annualRate / 12 / 100
	var fv, invested float64
	contribution := monthly
	for y := 0; y < years; y++ {
		for m := 0; m < 12; m++ {
			fv = (fv + contribution) * (1 + r)
			invested += contribution
		}
		contribution *= 1 + stepUpPct/100
	}
	return StepUpResult{FutureValue: fv, Invested: invested, Gain: fv - invested}, nil
}

// Lumpsum returns the future value of a one-off investment.
func Lumpsum(principal, annualRate, years float64) float64 {
	return principal * math.Pow(1+annualRate/100, years)
}

// CAGR returns the compound annual growth rate as a percentage.
func CAGR(initial, final, years float64) float64 {
	if initial <= 0 || years <= 0 {
		return 0
	}
	return (math.Pow(final/initial, 1/years) - 1) * 100
}

// ---------------------------------------------------------------------------
// GST
// ---------------------------------------------------------------------------

// GST is a base amount, its tax and the gross total.
type GST struct {
	Base  float64
	Tax   float64
	Total float64
}

// GSTAdd adds GST to a base amount.
func GSTAdd(amount, ratePct float64) GST {
	tax := amount * ratePct / 100
	return GST{Base: amount, Tax: tax, Total: amount + tax}
}

// GSTRemove strips GST out of a gross (GST-inclusive) amount.
func GSTRemove(grossAmount, ratePct float64) GST {
	base := grossAmount / (1 + ratePct/100)
	return GST{Base: base, Tax: grossAmount - base, Total: grossAmount}
}

// DefaultGSTInterestRate is the notified annual rate under Section 50(1).
const DefaultGSTInterestRate = 18.0

// GSTInterest returns interest on a late GST payment.
//
// Under Rule 88B(1) interest runs on the tax actually paid in cash from the
// electronic cash ledger — NOT on the gross output liability. Getting this
// wrong overstates the interest, often by several times. Pass cashTaxPaid as
// the cash-ledger portion, not the gross bill.
//
// Pass annualRatePct as DefaultGSTInterestRate unless the notified rate has
// moved.
func GSTInterest(cashTaxPaid float64, days int, annualRatePct float64) float64 {
	return cashTaxPaid * annualRatePct * float64(days) / 100 / 365
}

// ---------------------------------------------------------------------------
// Statutory — parameterised, because these move
// ---------------------------------------------------------------------------

// GratuityOptions carries the statutory figures that change by notification.
// The zero value means "use the defaults".
type GratuityOptions struct {
	Ceiling  float64 // 0 -> 20,00,000. Use 25,00,000 for Central Government civil employees.
	MinYears float64 // 0 -> 5. Use 1 for fixed-term employees.
}

// Gratuity is the computed entitlement.
type Gratuity struct {
	Eligible     bool
	Amount       float64
	FormulaValue float64
	Ceiling      float64
	Capped       bool
}

// ComputeGratuity applies the Code on Social Security, 2020 (in force
// 21 Nov 2025). lastSalary is the last drawn monthly Basic + DA; years is
// completed years of service.
func ComputeGratuity(lastSalary, years float64, opts GratuityOptions) Gratuity {
	ceiling := opts.Ceiling
	if ceiling == 0 {
		ceiling = 2000000
	}
	minYears := opts.MinYears
	if minYears == 0 {
		minYears = 5
	}
	if years < minYears {
		return Gratuity{Eligible: false, Ceiling: ceiling}
	}
	formulaValue := (15.0 / 26.0) * lastSalary * years
	return Gratuity{
		Eligible:     true,
		Amount:       math.Min(formulaValue, ceiling),
		FormulaValue: formulaValue,
		Ceiling:      ceiling,
		Capped:       formulaValue > ceiling,
	}
}

// Slab is one income-tax bracket: everything up to Upto is taxed at Rate.
// The final slab must use math.Inf(1) as Upto.
type Slab struct {
	Upto float64
	Rate float64
}

// TaxOptions overrides the statutory defaults. The zero value means
// "use the FY 2026-27 defaults".
type TaxOptions struct {
	Slabs      []Slab  // nil -> DefaultSlabs
	RebateUpto float64 // 0 -> 12,00,000 (Section 87A)
	Cess       float64 // 0 -> 0.04
}

// DefaultSlabs are the new-regime slabs for FY 2026-27 (AY 2027-28),
// unchanged by Budget 2026.
func DefaultSlabs() []Slab {
	return []Slab{
		{400000, 0}, {800000, 0.05}, {1200000, 0.10}, {1600000, 0.15},
		{2000000, 0.20}, {2400000, 0.25}, {math.Inf(1), 0.30},
	}
}

// Tax is the computed liability.
type Tax struct {
	Tax   float64
	Cess  float64
	Total float64
}

// IncomeTaxNewRegime computes tax under Section 115BAC, including the
// Section 87A rebate with marginal relief — tax cannot exceed the income above
// the rebate threshold, otherwise earning one rupee more would cost more than
// one rupee.
func IncomeTaxNewRegime(taxableIncome float64, opts TaxOptions) Tax {
	slabs := opts.Slabs
	if slabs == nil {
		slabs = DefaultSlabs()
	}
	rebateUpto := opts.RebateUpto
	if rebateUpto == 0 {
		rebateUpto = 1200000
	}
	cess := opts.Cess
	if cess == 0 {
		cess = 0.04
	}

	var tax, prev float64
	for _, s := range slabs {
		if taxableIncome > prev {
			tax += (math.Min(taxableIncome, s.Upto) - prev) * s.Rate
		}
		prev = s.Upto
		if taxableIncome <= s.Upto {
			break
		}
	}
	if taxableIncome <= rebateUpto {
		tax = 0
	} else {
		tax = math.Min(tax, taxableIncome-rebateUpto)
	}
	return Tax{Tax: tax, Cess: tax * cess, Total: tax * (1 + cess)}
}

// AnnualContributionMaturity returns the maturity of an annual-contribution
// scheme such as PPF or SSY.
// It returns an *ErrTenure if years exceeds MaxYears.
func AnnualContributionMaturity(yearlyAmount, annualRatePct float64, years int) (float64, error) {
	if err := checkYears(years); err != nil {
		return 0, err
	}
	var bal float64
	for y := 0; y < years; y++ {
		bal = (bal + yearlyAmount) * (1 + annualRatePct/100)
	}
	return bal, nil
}
