# indian-finance-formulas-go

Zero-dependency finance formulas for India — EMI, prepayment, SIP, step-up SIP,
GST, gratuity, income tax. Go port of
[`indian-finance-formulas`](https://github.com/javeed450-sudo/indian-finance-formulas).

```sh
go get github.com/javeed450-sudo/indian-finance-formulas-go
```

```go
import indianfinance "github.com/javeed450-sudo/indian-finance-formulas-go"

emi := indianfinance.EMI(3000000, 8.5, 240) // 26034.70
```

## Why it exists

Every function is extracted from the calculators running at
[emicalcs.com](https://emicalcs.com). The point of publishing them is that a
finance formula should be checkable, not taken on trust.

The test suite is a **direct port of the JavaScript one** — same 22 cases, same
expected values, same tolerances — so both implementations are held to a single
standard rather than drifting into two.

```sh
go test ./...
```

## Statutory values are options, not constants

Tax slabs, the gratuity ceiling and the GST interest rate change by
notification. Every one of them is a documented option with a default, so you
can update it the day it moves instead of waiting for a release:

```go
// Central Government civil employees: Rs 25,00,000 ceiling
indianfinance.ComputeGratuity(500000, 30, indianfinance.GratuityOptions{Ceiling: 2500000})

// your own slabs when they change
indianfinance.IncomeTaxNewRegime(1600000, indianfinance.TaxOptions{Slabs: mySlabs})
```

## Three things this gets right that most implementations do not

**GST interest runs on the cash-ledger portion.** Rule 88B(1) charges interest
on tax actually paid in cash, not on the gross output liability. Running it on
the gross bill overstates the interest — on the worked example in the tests, by
3.3×.

**The 87A rebate carries marginal relief.** Tax cannot exceed the income above
the rebate threshold. Without it, earning one rupee more than ₹12,00,000 would
cost more than one rupee.

**A step-up SIP does not beat a flat SIP of the same total outlay.** It wins in
headline terms only because more money goes in. Hold the money constant and the
flat schedule wins, because its rupees compound for longer. That is asserted in
the test suite, not just claimed here.

## Tenure ceiling

Schedule functions step one period at a time, so an absurd tenure is an
out-of-memory crash in the *caller's* process rather than a slow answer. Tenure
is capped at 1200 months / 100 years — far beyond the longest real home loan —
and exceeding it returns an `*ErrTenure` instead.

## Licence

MIT. Not financial advice.
