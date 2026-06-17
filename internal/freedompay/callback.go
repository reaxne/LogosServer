package freedompay

import (
	"errors"
	"math"
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
	amount, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(amount * 100)), nil
}
