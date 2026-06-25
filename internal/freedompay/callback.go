package freedompay

import (
	"errors"
	"strconv"
	"strings"
)

type Callback struct {
	OrderID     int64
	PaymentID   string
	AmountCents int64
	Currency    string
	Paid        bool
	Failed      bool
}

func ParseCallback(values map[string]string) (Callback, error) {
	orderID, err := strconv.ParseInt(values["pg_order_id"], 10, 64)
	if err != nil || orderID <= 0 {
		return Callback{}, errors.New("invalid order id")
	}
	amount, err := parseAmountCents(values["pg_amount"])
	if err != nil || amount <= 0 {
		return Callback{}, errors.New("invalid amount")
	}

	status := strings.ToLower(strings.TrimSpace(values["pg_payment_status"]))
	result := strings.ToLower(strings.TrimSpace(values["pg_result"]))
	return Callback{
		OrderID:     orderID,
		PaymentID:   values["pg_payment_id"],
		AmountCents: amount,
		Currency:    strings.TrimSpace(values["pg_currency"]),
		Paid:        result == "1" || status == "success" || status == "ok" || status == "paid",
		Failed:      result == "0" || status == "failed" || status == "failure" || status == "error",
	}, nil
}

func parseAmountCents(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, strconv.ErrSyntax
	}
	if strings.HasPrefix(value, "-") {
		return 0, strconv.ErrSyntax
	}

	whole, fraction, ok := strings.Cut(value, ".")
	if !ok {
		amount, err := strconv.ParseInt(whole, 10, 64)
		if err != nil {
			return 0, err
		}
		return amount * 100, nil
	}

	if whole == "" {
		whole = "0"
	}
	if len(fraction) > 2 {
		return 0, strconv.ErrSyntax
	}
	for len(fraction) < 2 {
		fraction += "0"
	}

	wholeAmount, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	fractionAmount, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, err
	}
	return wholeAmount*100 + fractionAmount, nil
}
