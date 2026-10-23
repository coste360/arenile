# Verificare la firma dei webhook

Ogni consegna dei webhook in uscita di Arenile (specifica 18.2) è firmata con il **segreto dell'endpoint**, mostrato
una sola volta quando crei l'endpoint o ne ruoti il segreto (**Configurazione → Webhook**). Il tuo server deve
verificare la firma prima di fidarsi del contenuto: senza la firma chiunque conosca l'URL potrebbe inviarti eventi
falsi. Questa pagina spiega il formato, i controlli da fare e contiene esempi pronti in quattro linguaggi e un
vettore di prova.

Per configurare gli endpoint, i tentativi e il registro delle consegne vedi [Integrazioni](integrazioni.md).

## Il formato

Ogni richiesta `POST` porta l'header:

```text
Arenile-Signature: t=1793530800,v1=72beef60030e2c3e1974ec9a61ee762e6a9a9a01f7bc4458bb6aa70c89591d73
```

- `t` è l'istante della firma in secondi Unix (UTC);
- `v1` è l'**HMAC-SHA256**, in esadecimale minuscolo, calcolato con il segreto dell'endpoint come chiave sul testo
  `<t>.<corpo grezzo>` (il valore di `t`, un punto, poi i byte del corpo esattamente come ricevuti);
- il segreto si usa **per intero**, prefisso `whsec_` compreso;
- durante il **periodo di grazia** di una rotazione del segreto l'header contiene **due** valori `v1`, uno per il
  vecchio e uno per il nuovo segreto: la consegna è valida se **uno qualsiasi** dei due coincide.

L'evento di prova inviato dal pannello (**Invia evento di prova**) è firmato nello stesso modo: usalo per collaudare
il ricevitore.

## I controlli da fare

1. **Usa il corpo grezzo.** Calcola l'HMAC sui byte ricevuti, prima di qualsiasi parsing JSON: riserializzare il JSON
   cambia spazi e ordine delle chiavi e la firma non torna più.
2. **Confronta a tempo costante** (`timingSafeEqual`, `hmac.compare_digest`, `hmac.Equal`, `hash_equals`), mai con
   `==`.
3. **Controlla l'ora.** Rifiuta le richieste con `t` più lontano di **5 minuti** dall'ora del tuo server: una richiesta
   intercettata non può essere ripresentata più tardi. Tieni l'orologio del server sincronizzato (NTP).
4. **Scarta i duplicati.** Le consegne sono *almeno una volta*: dopo un errore di rete o un reinvio dal registro lo
   stesso evento può arrivare di nuovo, con la stessa firma ricalcolata. Usa l'header `Arenile-Event-Id` (uguale a
   ogni tentativo) per ignorare quelli già elaborati.
5. **Rispondi presto.** Restituisci un codice `2xx` entro 10 secondi e fai il lavoro pesante dopo; una firma non
   valida merita un `400` (non verrà ritentata con successo, ma resta nel registro delle consegne).

Non registrare mai il segreto nei log e conservalo come una password (variabile d'ambiente o gestore di segreti).

## Esempi

### Node.js (Express)

```js
import crypto from "node:crypto";
import express from "express";

const SECRETS = [process.env.ARENILE_WEBHOOK_SECRET]; // whsec_... (aggiungi il nuovo durante una rotazione)

export function verifyArenile(header, rawBody, secrets = SECRETS, toleranceSec = 300) {
  const parts = header.split(",").map((p) => p.trim().split("="));
  const t = parts.find(([k]) => k === "t")?.[1];
  const sigs = parts.filter(([k]) => k === "v1").map(([, v]) => Buffer.from(v, "hex"));
  if (!t || sigs.length === 0) return false;
  if (Math.abs(Date.now() / 1000 - Number(t)) > toleranceSec) return false;
  return secrets.some((secret) => {
    const expected = crypto.createHmac("sha256", secret).update(`${t}.`).update(rawBody).digest();
    return sigs.some((s) => s.length === expected.length && crypto.timingSafeEqual(s, expected));
  });
}

const app = express();
app.post("/arenile", express.raw({ type: "application/json" }), (req, res) => {
  if (!verifyArenile(req.get("Arenile-Signature") ?? "", req.body)) return res.sendStatus(400);
  const event = JSON.parse(req.body.toString("utf8"));
  res.sendStatus(204); // rispondi subito, poi elabora event (scarta i duplicati per event.id)
});
```

### Python

```python
import hashlib
import hmac
import time


def verify_arenile(secret: str, header: str, raw_body: bytes, tolerance: int = 300) -> bool:
    items = [p.strip().split("=", 1) for p in header.split(",") if "=" in p]
    t = next((v for k, v in items if k == "t"), None)
    sigs = [v for k, v in items if k == "v1"]
    if t is None or not sigs or abs(time.time() - int(t)) > tolerance:
        return False
    expected = hmac.new(secret.encode(), f"{t}.".encode() + raw_body, hashlib.sha256).hexdigest()
    return any(hmac.compare_digest(s, expected) for s in sigs)
```

Con Flask il corpo grezzo è `request.get_data()`; con Django `request.body`.

### Go

```go
package arenilehook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"time"
)

// Verify reports whether header signs body with secret, within tolerance of now.
func Verify(secret, header string, body []byte, now time.Time, tolerance time.Duration) bool {
	var ts string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts = v
		case "v1":
			sigs = append(sigs, v)
		}
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return false
	}
	if math.Abs(now.Sub(time.Unix(sec, 0)).Seconds()) > tolerance.Seconds() {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(ts + "."))
	m.Write(body)
	want := m.Sum(nil)
	for _, s := range sigs {
		if got, err := hex.DecodeString(s); err == nil && hmac.Equal(got, want) {
			return true
		}
	}
	return false
}
```

### PHP

```php
<?php
function verify_arenile(string $secret, string $header, string $rawBody, int $tolerance = 300): bool {
    $t = null;
    $sigs = [];
    foreach (explode(',', $header) as $part) {
        [$k, $v] = array_pad(explode('=', trim($part), 2), 2, '');
        if ($k === 't') { $t = $v; }
        if ($k === 'v1') { $sigs[] = $v; }
    }
    if ($t === null || !$sigs || abs(time() - (int) $t) > $tolerance) {
        return false;
    }
    $expected = hash_hmac('sha256', $t . '.' . $rawBody, $secret);
    foreach ($sigs as $s) {
        if (hash_equals($expected, $s)) { return true; }
    }
    return false;
}

// $rawBody = file_get_contents('php://input');
// $ok = verify_arenile(getenv('ARENILE_WEBHOOK_SECRET'), $_SERVER['HTTP_ARENILE_SIGNATURE'] ?? '', $rawBody);
```

## Vettore di prova

Per controllare la tua implementazione senza un server Arenile, usa questi valori (fittizi: non sono segreti reali).
Ricorda di **disattivare il controllo dell'ora** o di fissare l'ora a `t`, perché il vettore è nel passato.

| Campo | Valore |
|---|---|
| Segreto | `whsec_esempio_non_usare` |
| Nuovo segreto (rotazione) | `whsec_nuovo_esempio` |
| `t` | `1793530800` (1 novembre 2026, 11:00 UTC) |
| Corpo (senza a capo finale) | `{"id":"evt_0192f1c4-7a5e-7b3a-9f0e-2c6d8e1a4b70","type":"webhook.test","establishment_id":"0192f1c4-0000-7000-8000-000000000001","occurred_at":"2026-11-01T11:00:00Z","data":{},"version":1}` |
| `v1` con il segreto | `72beef60030e2c3e1974ec9a61ee762e6a9a9a01f7bc4458bb6aa70c89591d73` |
| `v1` con il nuovo segreto | `0a64ef7020d9b85374109bc16b5b9d908c30f01191cb047a293508ea044e6ba5` |

Durante una rotazione l'header è quindi:

```text
Arenile-Signature: t=1793530800,v1=72beef60030e2c3e1974ec9a61ee762e6a9a9a01f7bc4458bb6aa70c89591d73,v1=0a64ef7020d9b85374109bc16b5b9d908c30f01191cb047a293508ea044e6ba5
```

Da riga di comando:

```sh
printf '%s' '1793530800.<corpo>' | openssl dgst -sha256 -hmac 'whsec_esempio_non_usare'
```

Lo stesso vettore è verificato dai test di Arenile (`TestSignature_DocumentedVector` in `internal/domain/event`):
se lo schema di firma cambiasse, il test fallirebbe insieme a questa pagina.

## Problemi frequenti

- **La firma non torna mai**: stai firmando il JSON riserializzato invece del corpo grezzo, oppure hai tolto il
  prefisso `whsec_` dal segreto, oppure un proxy davanti al tuo server modifica il corpo (compressione, a capo).
- **Funziona con l'evento di prova ma non con gli altri**: l'endpoint non è iscritto a quegli eventi, oppure è in
  pausa o disattivato per errori (vedi il registro delle consegne).
- **Errori dopo una rotazione**: finito il periodo di grazia resta valido solo il nuovo segreto; aggiorna il
  ricevitore entro quel termine.
- **Richieste rifiutate per l'ora**: l'orologio del tuo server è fuori sincronia; attiva NTP.
