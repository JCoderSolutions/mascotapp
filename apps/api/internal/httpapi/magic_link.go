package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gentleman/mascotapp/apps/api/internal/auth"
	"github.com/gentleman/mascotapp/apps/api/internal/db"
	"github.com/gentleman/mascotapp/apps/api/internal/email"
)

// magicLinkLifetime is what the mail tells the reader they have.
//
// It is NOT enforced anywhere yet, and that is stated rather than implied — see
// the note on MagicLinkHandler. Writing a number the system does not check would
// be worse than writing none, so this constant exists to be moved into the
// redemption path when that path is built, not to look like a control today.
const magicLinkLifetime = 15 * time.Minute

// magicLinkAccepted is the ONE response this endpoint gives.
//
// It is a fixed value, not a rendered message, because the requirement is that
// the reply carry no information about the address. A templated body is one
// edit away from "we sent a link to your account", which answers the question
// the endpoint exists not to answer.
var magicLinkAccepted = map[string]string{"status": "accepted"}

// MagicLinkHandler starts a passwordless sign-in for an address.
//
// The whole requirement is an ABSENCE: nothing in the reply may reveal whether
// the address has an account (proposal Decision 7). Three channels carry that
// information and each is closed deliberately:
//
//   - Status and body: one fixed response, written from a package-level value
//     so no branch can render a different one.
//   - The email: sent only for an address that exists, which is the point, and
//     is why the other two channels have to be silent — the inbox is the one
//     signal its owner is entitled to.
//   - Timing: the lookup runs identically on both paths, and the send does NOT
//     block the reply. A synchronous send would make the existing-account path
//     cost an SMTP round trip that the other path does not pay, which is an
//     oracle a stopwatch reads through two identical bodies.
//
// **What this does NOT do, stated plainly because the gap is invisible from
// here.** Phase 02 has no redemption endpoint and no table to hold these
// tokens: `T-02-031` is refresh, `T-02-032` is TOTP enrolment, and no task in
// the phase consumes a magic link. So the token this mails is unguessable and
// currently unredeemable, single use is not enforced, and neither is the
// lifetime the mail quotes. Storing a hash — the shape `totp_recovery_codes`
// already uses — is what buys both, and it belongs with the endpoint that
// redeems it, so the storage decision is deliberately left to that task rather
// than guessed at here.
func MagicLinkHandler(
	authPool db.Beginner,
	sender email.Sender,
	now func() time.Time,
	linkBase string,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "the request body is not valid JSON")
			return
		}
		// A malformed address is a property of the INPUT, which the caller
		// already knows, so refusing it reveals nothing. A 400 that happened
		// only for unregistered addresses would be a different matter.
		address := strings.TrimSpace(req.Email)
		if address == "" {
			badRequest(w, "email is required")
			return
		}

		ctx := r.Context()

		credentials, found, err := findCredentials(ctx, authPool, address)
		if err != nil {
			// Logged, and the caller still gets the ordinary reply. A 500 here
			// would fire only for addresses whose lookup failed, and the shape
			// of that failure could depend on the address.
			slog.ErrorContext(ctx, "magic link lookup failed", "error", err)
			acceptMagicLink(w)
			return
		}

		if found && credentials.Status == "active" {
			token, err := newMagicLinkToken()
			if err != nil {
				slog.ErrorContext(ctx, "generating a magic-link token", "error", err)
				acceptMagicLink(w)
				return
			}

			// context.WithoutCancel, not the request context. The reply is
			// written immediately below, which cancels r.Context(), and
			// email.checkSend refuses a cancelled context — so passing the
			// request's own context would turn every send into a no-op that
			// logged nothing and looked like it worked.
			go deliverMagicLink(context.WithoutCancel(ctx), sender, address, linkBase, token, now())
		}

		acceptMagicLink(w)
	})
}

// acceptMagicLink writes the one reply. 202 rather than 200: the server accepted
// the request, and says nothing about whether mail followed.
func acceptMagicLink(w http.ResponseWriter) {
	writeJSON(w, http.StatusAccepted, magicLinkAccepted)
}

// deliverMagicLink sends the mail on its own goroutine.
//
// A failure here is logged and goes no further. There is nobody left to tell:
// the reply is already written, and it would have been the same reply anyway.
func deliverMagicLink(
	ctx context.Context,
	sender email.Sender,
	address, linkBase, token string,
	issuedAt time.Time,
) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	link := strings.TrimSuffix(linkBase, "/") + "/" + token

	err := sender.Send(ctx, address, email.Message{
		Purpose: email.PurposeMagicLink,
		Subject: "Tu enlace de acceso a MascotApp",
		Body: fmt.Sprintf(
			"Entrá con este enlace:\n\n%s\n\nVence a las %s. Si no lo pediste, ignorá este correo.",
			link, issuedAt.Add(magicLinkLifetime).Format(time.RFC1123),
		),
	})
	if err != nil {
		// The address is deliberately NOT in this line. A log that pairs a
		// delivery failure with an address is the same existence oracle the
		// response spent three channels avoiding, moved somewhere with different
		// retention and a shipper pointed at it.
		slog.ErrorContext(ctx, "sending a magic link failed", "error", err)
	}
}

// newMagicLinkToken draws an unguessable token.
//
// It reuses auth.NewRefreshSecret for the randomness rather than calling
// crypto/rand here: that function is already the reviewed path for "one
// credential's worth", and a second one would be a second place to get the
// length wrong. Base64url because this ends up in a URL.
func newMagicLinkToken() (string, error) {
	secret, err := auth.NewRefreshSecret()
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(secret), nil
}
