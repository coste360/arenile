---
subject: Webhook disattivato: {{webhook.host}}
preheader: Disattivato dopo {{webhook.failures}} consegne consecutive non riuscite.
---
# Webhook disattivato

L'endpoint webhook verso **{{webhook.host}}** di {{establishment.name}} è stato disattivato automaticamente dopo {{webhook.failures}} consegne consecutive non riuscite. Gli eventi successivi non vengono più inviati.

Controlla il servizio che riceve i webhook, poi riattiva l'endpoint da Configurazione › Integrazioni: lo storico dei tentativi mostra la risposta di ogni invio.
