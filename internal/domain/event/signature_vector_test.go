package event

import (
	"testing"
	"time"
)

// The test vector published in docs/guida/webhook-verifica-firma.md: if the
// signature scheme changes, this test fails and the guide must change too.
const (
	docVectorBody    = `{"id":"evt_0192f1c4-7a5e-7b3a-9f0e-2c6d8e1a4b70","type":"webhook.test","establishment_id":"0192f1c4-0000-7000-8000-000000000001","occurred_at":"2026-11-01T11:00:00Z","data":{},"version":1}`
	docVectorTS      = 1793530800
	docVectorSecret1 = "whsec_esempio_non_usare"
	docVectorSecret2 = "whsec_nuovo_esempio"
	docVectorSig1    = "72beef60030e2c3e1974ec9a61ee762e6a9a9a01f7bc4458bb6aa70c89591d73"
	docVectorSig2    = "0a64ef7020d9b85374109bc16b5b9d908c30f01191cb047a293508ea044e6ba5"
)

func TestSignature_DocumentedVector(t *testing.T) {
	at := time.Unix(docVectorTS, 0).UTC()
	body := []byte(docVectorBody)

	single := "t=1793530800,v1=" + docVectorSig1
	if got := Sign(docVectorSecret1, body, at); got != single {
		t.Fatalf("Sign = %q, want %q", got, single)
	}
	rotated := single + ",v1=" + docVectorSig2
	if got := SignAll([]string{docVectorSecret1, docVectorSecret2}, body, at); got != rotated {
		t.Fatalf("SignAll = %q, want %q", got, rotated)
	}

	now := at.Add(2 * time.Minute)
	for _, secret := range []string{docVectorSecret1, docVectorSecret2} {
		if err := VerifySignature(secret, rotated, body, now, 5*time.Minute); err != nil {
			t.Fatalf("VerifySignature(%s): %v", secret, err)
		}
	}
	if err := VerifySignature("whsec_altro", rotated, body, now, 5*time.Minute); err == nil {
		t.Fatal("a wrong secret was accepted")
	}
	if err := VerifySignature(docVectorSecret1, single, body, at.Add(10*time.Minute), 5*time.Minute); err == nil {
		t.Fatal("a stale timestamp was accepted")
	}
	if err := VerifySignature(docVectorSecret1, single, append(body, ' '), now, 5*time.Minute); err == nil {
		t.Fatal("a modified body was accepted")
	}
}
