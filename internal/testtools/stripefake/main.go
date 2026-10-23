// Command stripefake runs the fake Stripe API of package stripetest as a
// process, with its hosted payment page, for the end-to-end tests and the
// real-binary acceptance (never for production). Point Arenile at it with
// ARENILE_STRIPE_API_URL and configure the establishment with the fake
// credentials stripetest.SecretKey / stripetest.WebhookSecret.
//
//	go run ./internal/testtools/stripefake -addr 127.0.0.1:12111 -arenile http://127.0.0.1:8090
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/coste360/arenile/internal/domain/payment/stripe/stripetest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:12111", "listen address")
	arenile := flag.String("arenile", "http://127.0.0.1:8090", "Arenile base URL (webhook target)")
	flag.Parse()
	s, err := stripetest.Start(*addr, stripetest.StandaloneOptions{ArenileBaseURL: *arenile})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer s.Close()
	fmt.Printf("fake Stripe on %s (webhooks to %s)\n", s.URL, *arenile)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}
