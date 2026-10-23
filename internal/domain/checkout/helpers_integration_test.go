//go:build integration

package checkout_test

import (
	"errors"

	"github.com/coste360/arenile/internal/domain/checkout"
	"github.com/coste360/arenile/internal/domain/checkout/checkouttest"
)

const slug = checkouttest.Slug

func isNotFound(err error) bool { return errors.Is(err, checkout.ErrSessionNotFound) }
