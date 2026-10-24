package problem

import (
	"context"
	"errors"
	"net/http"
)

// StatusClientClosedRequest is the status recorded when the client went
// away before the response (nginx's 499): the request was not served, but
// it is not a server error, so it is neither logged as one nor counted in
// the 5xx metrics.
const StatusClientClosedRequest = 499

// ClientGone reports whether err comes from the client cancelling the
// request: context.Canceled in the error chain, or a request context that
// is already cancelled (drivers do not always wrap the cause). A server
// side deadline (context.DeadlineExceeded) is still a server error.
func ClientGone(r *http.Request, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return r != nil && errors.Is(r.Context().Err(), context.Canceled)
}
