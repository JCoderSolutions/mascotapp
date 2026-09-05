package email_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gentleman/mascotapp/apps/api/internal/email"
)

// The email port (email-delivery / *Email is sent through a port with no
// dependency on a concrete provider*, and proposal decision 2).
//
// The point of this package is what it does NOT do. The auth flow has to be
// end-to-end testable with no provider account, no API key and no outbound
// call, so the only adapter this phase ships is a logging stub. Wiring a real
// provider is a later, unassigned task.
//
// Two properties carry the weight here, and both are asserted rather than
// asserted-about-in-a-comment:
//
//  1. the stub cannot reach the network — not "did not this time", but cannot,
//     because the package does not import anything that could;
//  2. the stub never logs the body — a magic-link body carries a live
//     credential, and a log line is the last place it should end up.

// -----------------------------------------------------------------------------
// Test helpers — the second adapter, and a caller that only knows the port.
// -----------------------------------------------------------------------------

// recordedSend is what the test double keeps: everything, unlike the stub.
type recordedSend struct {
	to  string
	msg email.Message
}

// recordingSender is the second adapter of the substitution scenario. It lives
// in the test file on purpose: a test double that ships in the production
// package is a production dependency wearing a costume.
type recordingSender struct {
	sends []recordedSend
	err   error
}

func (r *recordingSender) Send(_ context.Context, to string, msg email.Message) error {
	if r.err != nil {
		return r.err
	}
	r.sends = append(r.sends, recordedSend{to: to, msg: msg})
	return nil
}

// magicLinkBody is a stand-in for the real thing: a URL carrying a live
// single-use credential. If this string ever appears in a log, the credential
// appears in the log.
const magicLinkBody = "https://mascotapp.example/auth/magic?token=SUPER-SECRET-LIVE-CREDENTIAL"

// sendMagicLink is the caller. It depends on the port and on nothing else —
// which is the whole substitution scenario in one function signature.
func sendMagicLink(ctx context.Context, sender email.Sender, to string) error {
	return sender.Send(ctx, to, email.Message{
		Purpose: email.PurposeMagicLink,
		Subject: "Your sign-in link",
		Body:    magicLinkBody,
	})
}

// newBufferedLogger returns a JSON logger writing into the returned buffer, so
// a test can read back exactly what the adapter recorded.
func newBufferedLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, buf
}

// decodeRecords parses every line the handler wrote. It fails the test on an
// unparseable line rather than skipping it, because a line that cannot be
// parsed is a line whose contents were never checked.
func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("the adapter wrote a line that is not JSON: %q (%v)", line, err)
		}
		records = append(records, record)
	}
	return records
}

// -----------------------------------------------------------------------------
// Scenario: a magic-link email is recorded by the stub, not sent over the
// network.
// -----------------------------------------------------------------------------

func TestLogSenderRecordsTheRecipientAndThePurpose(t *testing.T) {
	logger, buf := newBufferedLogger()
	sender := email.NewLogSender(logger)

	if err := sendMagicLink(t.Context(), sender, "adopter@example.test"); err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}

	records := decodeRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("expected exactly one record, got %d: %s", len(records), buf.String())
	}

	if got := records[0]["recipient"]; got != "adopter@example.test" {
		t.Errorf("recipient = %v, want adopter@example.test", got)
	}
	if got := records[0]["purpose"]; got != string(email.PurposeMagicLink) {
		t.Errorf("purpose = %v, want %s", got, email.PurposeMagicLink)
	}
}

func TestLogSenderNeverLogsTheMessageBody(t *testing.T) {
	logger, buf := newBufferedLogger()
	sender := email.NewLogSender(logger)

	if err := sendMagicLink(t.Context(), sender, "adopter@example.test"); err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}

	// Not "the body field is absent" — the whole output, whatever shape the
	// adapter chose to write it in.
	if strings.Contains(buf.String(), "SUPER-SECRET-LIVE-CREDENTIAL") {
		t.Fatalf("the magic-link credential reached the log: %s", buf.String())
	}
	if strings.Contains(buf.String(), magicLinkBody) {
		t.Fatalf("the message body reached the log: %s", buf.String())
	}
}

// failingTransport fails the test if anything routes an HTTP request through
// the default client while it is installed.
type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("the adapter attempted an outbound HTTP call to %s", r.URL)
	return nil, errors.New("no outbound call is allowed from the logging stub")
}

func TestLogSenderMakesNoOutboundHTTPCall(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = failingTransport{t: t}
	t.Cleanup(func() { http.DefaultTransport = original })

	logger, _ := newBufferedLogger()
	if err := sendMagicLink(t.Context(), email.NewLogSender(logger), "adopter@example.test"); err != nil {
		t.Fatalf("Send returned an error: %v", err)
	}
}

// TestTheEmailPackageImportsNothingThatCanReachTheNetwork is the structural
// half of the same guarantee. Swapping the default transport only proves that
// THIS call did not go out through THAT client; a hand-built client, or a
// shell-out to sendmail, would sail past it.
//
// Reading the imports proves the stronger thing: the package has no way to
// reach the network at all. When a real provider adapter arrives it will need
// its own package, or this test, and the design decision behind it, will have
// to be revisited on purpose rather than by accident.
func TestTheEmailPackageImportsNothingThatCanReachTheNetwork(t *testing.T) {
	forbidden := map[string]string{
		"net":      "raw sockets",
		"net/http": "an HTTP client",
		"net/smtp": "an SMTP client",
		"os/exec":  "shelling out to a mail transfer agent",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	parsed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(".", name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		parsed++

		for _, imported := range file.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if reason, isForbidden := forbidden[path]; isForbidden {
				t.Errorf("%s imports %q — that is %s, and the stub adapter must have no way to reach the network", name, path, reason)
			}
		}
	}

	// Without this the test passes just as loudly over an empty directory,
	// which is the failure mode where a green run proves nothing at all.
	if parsed < 2 {
		t.Fatalf("expected at least the port and the stub adapter, parsed %d non-test file(s)", parsed)
	}
}

// -----------------------------------------------------------------------------
// Scenario: the port accepts a second adapter without changing the caller.
// -----------------------------------------------------------------------------

func TestThePortAcceptsASecondAdapterWithNoChangeToTheCaller(t *testing.T) {
	logger, buf := newBufferedLogger()

	// The exact same call site, twice, over two unrelated implementations.
	adapters := map[string]email.Sender{
		"log sender":  email.NewLogSender(logger),
		"test double": &recordingSender{},
	}

	for name, adapter := range adapters {
		t.Run(name, func(t *testing.T) {
			if err := sendMagicLink(t.Context(), adapter, "adopter@example.test"); err != nil {
				t.Fatalf("Send returned an error: %v", err)
			}
		})
	}

	// And the double really saw the send — including the body the stub refuses
	// to log, which is the point of substituting it in the first place.
	double, ok := adapters["test double"].(*recordingSender)
	if !ok {
		t.Fatal("the test double is not a *recordingSender")
	}
	if len(double.sends) != 1 {
		t.Fatalf("the double recorded %d sends, want 1", len(double.sends))
	}
	if double.sends[0].to != "adopter@example.test" {
		t.Errorf("the double recorded recipient %q", double.sends[0].to)
	}
	if double.sends[0].msg.Body != magicLinkBody {
		t.Errorf("the double recorded body %q, want the magic-link body", double.sends[0].msg.Body)
	}
	if buf.Len() == 0 {
		t.Error("the log sender wrote nothing")
	}
}

// -----------------------------------------------------------------------------
// The contract every adapter owes the caller.
// -----------------------------------------------------------------------------

func TestLogSenderRefusesAMessageWithNoRecipient(t *testing.T) {
	logger, buf := newBufferedLogger()

	err := email.NewLogSender(logger).Send(t.Context(), "", email.Message{Purpose: email.PurposeMagicLink})
	if !errors.Is(err, email.ErrNoRecipient) {
		t.Fatalf("Send with no recipient returned %v, want ErrNoRecipient", err)
	}
	if buf.Len() != 0 {
		t.Errorf("a refused send was recorded anyway: %s", buf.String())
	}
}

func TestLogSenderRefusesAMessageWithNoPurpose(t *testing.T) {
	logger, buf := newBufferedLogger()

	err := email.NewLogSender(logger).Send(t.Context(), "adopter@example.test", email.Message{})
	if !errors.Is(err, email.ErrNoPurpose) {
		t.Fatalf("Send with no purpose returned %v, want ErrNoPurpose", err)
	}
	if buf.Len() != 0 {
		t.Errorf("a refused send was recorded anyway: %s", buf.String())
	}
}

func TestLogSenderRefusesASendOnACancelledContext(t *testing.T) {
	logger, buf := newBufferedLogger()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := sendMagicLink(ctx, email.NewLogSender(logger), "adopter@example.test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send on a cancelled context returned %v, want context.Canceled", err)
	}
	// A caller that gave up before the send must not read a recorded send and
	// conclude the address was mailed.
	if buf.Len() != 0 {
		t.Errorf("a cancelled send was recorded anyway: %s", buf.String())
	}
}

func TestNewLogSenderWithoutALoggerStillSends(t *testing.T) {
	// A nil logger is a wiring mistake, not a reason to panic in the middle of
	// a login flow. It falls back to the process default.
	if err := sendMagicLink(t.Context(), email.NewLogSender(nil), "adopter@example.test"); err != nil {
		t.Fatalf("Send with a nil logger returned an error: %v", err)
	}
}
