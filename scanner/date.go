package scanner

import (
	"bufio"
	"errors"
	"net/mail"
	"os"
	"strings"
	"time"
)

func ParseDateInput(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func ParseMessageDate(emlPath string) (*time.Time, error) {
	f, err := os.Open(emlPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	msg, err := mail.ReadMessage(reader)
	if err != nil {
		return nil, err
	}
	dateHeader := strings.TrimSpace(msg.Header.Get("Date"))
	if dateHeader == "" {
		return nil, errors.New("message date header not found")
	}
	parsed, err := mail.ParseDate(dateHeader)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func DateInRange(messageDate time.Time, startDate, endDate *time.Time) bool {
	msgDay := messageDate.Format("2006-01-02")
	if startDate != nil {
		if msgDay < startDate.Format("2006-01-02") {
			return false
		}
	}
	if endDate != nil {
		if msgDay > endDate.Format("2006-01-02") {
			return false
		}
	}
	return true
}
