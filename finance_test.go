// A direct port of the JavaScript suite in ../indian-finance-formulas/test.
// Same cases, same expected values, same tolerances — so the two
// implementations are held to one standard rather than two.
package indianfinance

import (
	"errors"
	"math"
	"testing"
)

func near(t *testing.T, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("expected ~%v (tol %v), got %v", want, tol, got)
	}
}

func mustTenureErr(t *testing.T, err error) {
	t.Helper()
	var te *ErrTenure
	if !errors.As(err, &te) {
		t.Fatalf("expected *ErrTenure, got %v", err)
	}
}

// --- loans -----------------------------------------------------------------

func TestEMI(t *testing.T) {
	near(t, EMI(3000000, 8.5, 240), 26034.7, 0.5)
}

func TestEMIZeroRateSpreadsPrincipalEvenly(t *testing.T) {
	near(t, EMI(120000, 0, 12), 10000, 0.001)
}

func TestAmortisationRepaysTheLoan(t *testing.T) {
	rows, err := Amortisation(3000000, 8.5, 240)
	if err != nil {
		t.Fatal(err)
	}
	var paid float64
	for _, r := range rows {
		paid += r.Principal
	}
	near(t, paid, 3000000, 1)
	near(t, rows[len(rows)-1].Closing, 0, 0.01)
}

func TestAmortisationInterestMatchesTotalInterest(t *testing.T) {
	rows, err := Amortisation(1000000, 9, 120)
	if err != nil {
		t.Fatal(err)
	}
	var i float64
	for _, r := range rows {
		i += r.Interest
	}
	near(t, i, TotalInterest(1000000, 9, 120), 1)
}

func TestPrepaymentExtraMonthly(t *testing.T) {
	r, err := Prepayment(PrepaymentInput{Principal: 3000000, AnnualRate: 8.5, Months: 240, MonthlyExtra: 5000})
	if err != nil {
		t.Fatal(err)
	}
	near(t, r.InterestSaved, 1173056, 50)
	if r.MonthsSaved != 76 {
		t.Fatalf("monthsSaved: want 76, got %d", r.MonthsSaved)
	}
	if r.NewMonths != 164 {
		t.Fatalf("newMonths: want 164, got %d", r.NewMonths)
	}
}

func TestPrepaymentNoneSavesNothing(t *testing.T) {
	r, err := Prepayment(PrepaymentInput{Principal: 1000000, AnnualRate: 9, Months: 120})
	if err != nil {
		t.Fatal(err)
	}
	near(t, r.InterestSaved, 0, 1)
	if r.MonthsSaved != 0 {
		t.Fatalf("monthsSaved: want 0, got %d", r.MonthsSaved)
	}
}

// --- tenure ceiling --------------------------------------------------------

func TestAbsurdTenureErrorsInsteadOfExhaustingMemory(t *testing.T) {
	// Every schedule function steps one period at a time. Before the ceiling,
	// the JS original killed the node process with a heap OOM on 1e9 months.
	// (The JS suite also asserts on Infinity; int months makes that
	// unrepresentable here, which is the point of using int.)
	_, err := Amortisation(3000000, 0, 1e9)
	mustTenureErr(t, err)
	_, err = Prepayment(PrepaymentInput{Principal: 3000000, AnnualRate: 0, Months: 1e9})
	mustTenureErr(t, err)
	_, err = StepUpSIP(10000, 12, 1e9, 10)
	mustTenureErr(t, err)
	_, err = AnnualContributionMaturity(150000, 7.1, 1e9)
	mustTenureErr(t, err)
}

func TestCeilingSitsFarAboveAnyRealTenure(t *testing.T) {
	// 100 years must still compute — the longest home loan on offer is 30-40.
	rows, err := Amortisation(3000000, 8.5, 1200)
	if err != nil || len(rows) != 1200 {
		t.Fatalf("want 1200 rows, got %d (err %v)", len(rows), err)
	}
	p, err := Prepayment(PrepaymentInput{Principal: 3000000, AnnualRate: 8.5, Months: 1200})
	if err != nil || p.NewMonths != 1200 {
		t.Fatalf("want newMonths 1200, got %d (err %v)", p.NewMonths, err)
	}
	s, err := StepUpSIP(10000, 12, 100, 10)
	if err != nil || s.FutureValue <= 0 {
		t.Fatalf("want positive future value, got %v (err %v)", s.FutureValue, err)
	}
	m, err := AnnualContributionMaturity(150000, 7.1, 100)
	if err != nil || m <= 0 {
		t.Fatalf("want positive maturity, got %v (err %v)", m, err)
	}
}

// --- investments -----------------------------------------------------------

func TestSIPFutureValue(t *testing.T) {
	near(t, SIPFutureValue(10000, 12, 240), 9993378, 2000)
}

func TestSIPZeroRateIsJustTheContributions(t *testing.T) {
	near(t, SIPFutureValue(5000, 0, 24), 120000, 0.01)
}

func TestFlatSIPBeatsStepUpOnEqualOutlay(t *testing.T) {
	// The headline claim everywhere is that step-up wins. It only wins because
	// more money goes in. Hold the money constant and it reverses.
	step, err := StepUpSIP(10000, 12, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	flatMonthly := step.Invested / 240
	flat := SIPFutureValue(flatMonthly, 12, 240)
	if flat <= step.FutureValue {
		t.Fatalf("flat %.0f should beat step-up %.0f on equal outlay", flat, step.FutureValue)
	}
}

func TestCAGR(t *testing.T) {
	near(t, CAGR(100000, 250000, 5), 20.11, 0.01)
}

func TestCAGRAndLumpsumAreInverses(t *testing.T) {
	rate := CAGR(100000, 250000, 5)
	near(t, Lumpsum(100000, rate, 5), 250000, 1)
}

// --- GST -------------------------------------------------------------------

func TestGSTRoundTrip(t *testing.T) {
	a := GSTAdd(1000, 18)
	near(t, a.Total, 1180, 0.01)
	near(t, GSTRemove(a.Total, 18).Base, 1000, 0.01)
}

func TestGSTInterestRunsOnTheCashLedgerPortion(t *testing.T) {
	// Rs 30,000 cash paid, 30 days late, 18%.
	near(t, GSTInterest(30000, 30, DefaultGSTInterestRate), 443.84, 0.01)
	// The common error — running it on a Rs 1,00,000 gross bill — is 3.3x bigger.
	near(t, GSTInterest(100000, 30, DefaultGSTInterestRate), 1479.45, 0.01)
}

// --- statutory -------------------------------------------------------------

func TestGratuityTenYears(t *testing.T) {
	g := ComputeGratuity(50000, 10, GratuityOptions{})
	near(t, g.Amount, 288461.54, 0.01)
	if g.Capped {
		t.Fatal("should not be capped")
	}
}

func TestGratuityCeilings(t *testing.T) {
	if got := ComputeGratuity(500000, 30, GratuityOptions{}).Amount; got != 2000000 {
		t.Fatalf("want 2000000, got %v", got)
	}
	if got := ComputeGratuity(500000, 30, GratuityOptions{Ceiling: 2500000}).Amount; got != 2500000 {
		t.Fatalf("want 2500000, got %v", got)
	}
}

func TestGratuityMinimumService(t *testing.T) {
	if ComputeGratuity(50000, 4, GratuityOptions{}).Eligible {
		t.Fatal("4 years should not be eligible")
	}
	// ...unless fixed-term, where 1 year qualifies.
	if !ComputeGratuity(50000, 4, GratuityOptions{MinYears: 1}).Eligible {
		t.Fatal("fixed-term 4 years should be eligible")
	}
}

func TestIncomeTaxNilUpToTwelveLakh(t *testing.T) {
	if got := IncomeTaxNewRegime(1200000, TaxOptions{}).Tax; got != 0 {
		t.Fatalf("want 0, got %v", got)
	}
}

func TestIncomeTaxSixteenLakh(t *testing.T) {
	r := IncomeTaxNewRegime(1600000, TaxOptions{})
	near(t, r.Tax, 120000, 1)
	near(t, r.Total, 124800, 1)
}

func TestIncomeTaxMarginalReliefJustAboveTwelveLakh(t *testing.T) {
	// At Rs 12,10,000 the tax must not exceed the Rs 10,000 of income above the
	// threshold — otherwise earning Rs 1 more would cost more than Rs 1.
	if got := IncomeTaxNewRegime(1210000, TaxOptions{}).Tax; got > 10000 {
		t.Fatalf("marginal relief failed: tax %v exceeds 10000", got)
	}
}

func TestAnnualContributionMaturityPPF(t *testing.T) {
	m, err := AnnualContributionMaturity(150000, 7.1, 15)
	if err != nil {
		t.Fatal(err)
	}
	near(t, m, 4067053, 2000)
}
