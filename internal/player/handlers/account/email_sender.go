package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avdo/goeoserv/internal/config"
)

var ErrEmailTransportUnavailable = errors.New("email transport unavailable")

type Sender interface {
	Status() SenderStatus
	SendAccountValidation(context.Context, ValidationEmail) error
	SendRecoveryPIN(context.Context, RecoveryEmail) error
}

type SenderStatus struct {
	Configured bool
	Ready      bool
	Reason     string
}

type ValidationEmail struct {
	AccountName string
	Email       string
}

type RecoveryEmail struct {
	AccountName string
	Email       string
	PIN         string
	ExpiresIn   time.Duration
}

type noopSender struct {
	status SenderStatus
}

func NewSender(cfg *config.Config) Sender {
	return noopSender{status: senderStatus(cfg)}
}

func (s noopSender) Status() SenderStatus {
	return s.status
}

func (s noopSender) SendAccountValidation(context.Context, ValidationEmail) error {
	return fmt.Errorf("%w: %s", ErrEmailTransportUnavailable, s.status.Reason)
}

func (s noopSender) SendRecoveryPIN(context.Context, RecoveryEmail) error {
	return fmt.Errorf("%w: %s", ErrEmailTransportUnavailable, s.status.Reason)
}

func senderStatus(cfg *config.Config) SenderStatus {
	if cfg == nil {
		return SenderStatus{Reason: "server config is unavailable"}
	}

	if cfg.SMTP.Host == "" || cfg.SMTP.Port <= 0 || cfg.SMTP.FromAddress == "" {
		return SenderStatus{Reason: "SMTP is not configured"}
	}

	return SenderStatus{
		Configured: true,
		Reason:     "SMTP is configured but no outbound mail transport is implemented yet",
	}
}
