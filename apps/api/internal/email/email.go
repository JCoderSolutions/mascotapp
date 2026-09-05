// Package email is the outbound-email port and the only adapter this phase
// ships: a logging stub.
//
// email-delivery / *Email is sent through a port with no dependency on a
// concrete provider*, and proposal decision 2. The application layer depends on
// [Sender] and on nothing else, so the auth flow — magic link above all — is
// end-to-end testable with no provider account, no API key, and no outbound
// call. A real provider is a later adapter and a later task.
//
// The package deliberately imports nothing that can reach the network, and a
// test asserts that by reading these files' imports. When a provider adapter
// arrives it needs its own package, or that decision gets revisited on purpose
// instead of by accident.
package email

import (
	"context"
	"errors"
)

// Purpose names why an email is being sent.
//
// A closed set, not free text: it is the one field the logging stub is allowed
// to record alongside the address, so it has to be something an operator can
// group by — and something that cannot accidentally carry a subject line, a
// token, or a name into a log.
//
// Add a constant here when a task actually sends that kind of mail. An
// enumeration of mail nobody sends is a list of lies with a compiler behind it.
type Purpose string

// PurposeMagicLink is the passwordless sign-in link (§5.2). The only purpose
// Phase 02 sends.
const PurposeMagicLink Purpose = "magic_link"

// Message is one outbound email, already rendered.
//
// Rendering lives with the caller: the port carries text, not templates, so a
// provider adapter never becomes the place where copy is decided.
type Message struct {
	// Purpose is why this is being sent. Required.
	Purpose Purpose

	// Subject and Body are the rendered content. Body may carry a live
	// credential — a magic-link URL is a single-use credential in a string —
	// so no adapter may log it. See [LogSender].
	Subject string
	Body    string
}

// Sender is the port. One method, so the stub is honest about what it is and a
// provider adapter has nowhere to hide extra surface.
//
// Implementations MUST refuse a message with no recipient ([ErrNoRecipient]) or
// no purpose ([ErrNoPurpose]) rather than sending it, MUST honour ctx
// cancellation, and MUST NOT log Message.Body.
type Sender interface {
	Send(ctx context.Context, to string, msg Message) error
}

// The refusals every adapter owes the caller. They are sentinel errors because
// a magic-link handler has to tell "this address is unroutable" apart from
// "the provider is down" without reading error strings.
var (
	// ErrNoRecipient is returned for an empty destination address. Silently
	// dropping such a send is worse than failing it: the caller would go on to
	// tell the user their link is on its way.
	ErrNoRecipient = errors.New("email: the message has no recipient")

	// ErrNoPurpose is returned for a message with no [Purpose]. Without it the
	// send is unattributable in the logs, which is the one thing the stub
	// exists to provide.
	ErrNoPurpose = errors.New("email: the message has no purpose")
)

// checkSend applies the contract above. Shared so a second adapter in this
// package refuses exactly what the stub refuses, rather than approximately.
func checkSend(ctx context.Context, to string, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if to == "" {
		return ErrNoRecipient
	}
	if msg.Purpose == "" {
		return ErrNoPurpose
	}
	return nil
}
