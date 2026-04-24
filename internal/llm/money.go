package llm

import "fmt"

type Currency string

const (
	CurrencyCNY Currency = "CNY"
	CurrencyUSD Currency = "USD"
)

type Money struct {
	Amount   float64
	Currency Currency
}

func (m Money) IsZero() bool {
	return m.Amount == 0 || m.Currency == ""
}

func (m Money) Add(other Money) Money {
	if m.IsZero() {
		return other
	}
	if other.IsZero() {
		return m
	}
	if m.Currency != other.Currency {
		panic(fmt.Sprintf("cannot add %s to %s", m.Currency, other.Currency))
	}
	return Money{
		Amount:   m.Amount + other.Amount,
		Currency: m.Currency,
	}
}
