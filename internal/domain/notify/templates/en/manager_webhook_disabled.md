---
subject: Webhook disabled: {{webhook.host}}
preheader: Disabled after {{webhook.failures}} consecutive failed deliveries.
---
# Webhook disabled

The webhook endpoint to **{{webhook.host}}** of {{establishment.name}} was disabled automatically after {{webhook.failures}} consecutive failed deliveries. Later events are no longer sent.

Check the service receiving the webhooks, then reactivate the endpoint in Settings › Integrations: the attempt history shows the response of every delivery.
