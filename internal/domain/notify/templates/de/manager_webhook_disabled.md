---
subject: Webhook deaktiviert: {{webhook.host}}
preheader: Nach {{webhook.failures}} aufeinanderfolgenden fehlgeschlagenen Zustellungen deaktiviert.
---
# Webhook deaktiviert

Der Webhook-Endpunkt zu **{{webhook.host}}** von {{establishment.name}} wurde nach {{webhook.failures}} aufeinanderfolgenden fehlgeschlagenen Zustellungen automatisch deaktiviert. Weitere Ereignisse werden nicht mehr gesendet.

Prüfen Sie den Dienst, der die Webhooks empfängt, und aktivieren Sie den Endpunkt dann unter Einstellungen › Integrationen wieder: Der Verlauf der Versuche zeigt die Antwort jeder Zustellung.
