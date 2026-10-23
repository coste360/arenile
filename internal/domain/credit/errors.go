package credit

import (
	"errors"
	"fmt"

	"github.com/coste360/arenile/internal/domain/configerr"
)

// ErrInsufficient: the movement would make the balance negative
// (problem credit_insufficient, 422).
var ErrInsufficient = errors.New("credit: insufficient balance")

// ErrCustomerNotFound: unknown customer in the establishment (404).
var ErrCustomerNotFound = fmt.Errorf("%w: cliente", configerr.ErrNotFound)
