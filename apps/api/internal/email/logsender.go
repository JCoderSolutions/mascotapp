package email

import (
	"context"
	"log/slog"
)

// LogSender is the adapter this phase configures: it records the intended send
// and delivers nothing.
//
// It exists so the whole auth flow runs — and is asserted end to end — with no
// provider account and no network. A test reads these records to prove that a
// magic-link request for a known address mailed exactly one address, and that a
// request for an unknown one mailed none.
//
// # It records the recipient, and that is a deliberate exception
//
// §5.4 says logs never carry PII, and an address is PII. This adapter writes
// one anyway, because a stub that records "an email was sent to somebody" makes
// the flow unobservable and the tests above impossible.
//
// The exception is bounded by the adapter never being the production one: the
// moment a real provider is wired, LogSender stops being configured and the
// addresses stop being written. It is a development and test adapter. Naming it
// as such here is the guard, since nothing in the type system enforces it.
//
// What it never records is [Message.Body]. A magic-link body carries a live
// single-use credential, and a log line is a copy of that credential in a place
// with different retention, different access control, and a shipper pointed at
// it.
type LogSender struct {
	logger *slog.Logger
}

// NewLogSender returns a [LogSender] writing to logger.
//
// A nil logger falls back to [slog.Default] rather than panicking: a wiring
// mistake here would otherwise take down a login flow at the last step, which
// is a far worse failure than a record landing on the default handler.
func NewLogSender(logger *slog.Logger) *LogSender {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogSender{logger: logger}
}

// Send records the intended delivery and returns. It makes no outbound call.
//
// It refuses before recording, never after: a caller reading these records must
// never see one for a send that did not happen.
func (s *LogSender) Send(ctx context.Context, to string, msg Message) error {
	if err := checkSend(ctx, to, msg); err != nil {
		return err
	}

	// Recipient and purpose only. body_bytes is the length, which answers "did
	// we render an empty email?" without reproducing what was in it.
	s.logger.LogAttrs(ctx, slog.LevelInfo, "email send recorded",
		slog.String("adapter", "log"),
		slog.String("recipient", to),
		slog.String("purpose", string(msg.Purpose)),
		slog.Int("body_bytes", len(msg.Body)),
	)
	return nil
}

// LogSender is a Sender. Asserted here so a signature drift is a compile error
// in this file rather than a puzzle at the wiring site.
var _ Sender = (*LogSender)(nil)
