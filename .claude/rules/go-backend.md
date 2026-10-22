---
paths:
  - "**/*.go"
---
- Errors: wrap with `fmt.Errorf("context: %w", err)`; domain errors are typed
  sentinel values in each package (`ErrSpotUnavailable`, `ErrNoPrice`, ...).
- `context.Context` is the first parameter of every function that does I/O.
- Logging only with `log/slog`; never log emails, phones or tokens in clear.
- Tests are table-driven, in the same package, named `TestXxx_Scenario`.
- No global state; dependencies are passed through constructors.
- Time comes from an injected `clock.Clock` (`internal/clock`); use `clock.Fake` in tests.
- Money is `money.Cents` (`int64`); calendar dates are `calendar.Date`; slots use `calendar.Slot`/`calendar.Slots`.
- IDs are UUIDv7 (`uuid.NewV7()`).
- DB integration tests use `//go:build integration`; PostgreSQL only if `ARENILE_TEST_POSTGRES_URL` is set.
- Concurrency tests are named `TestConcurrency_*` and must pass with `-race`.
- Never edit `*.gen.go`; change `api/openapi.yaml` and run `make gen`.
