---
subject: Webhook désactivé : {{webhook.host}}
preheader: Désactivé après {{webhook.failures}} livraisons consécutives en échec.
---
# Webhook désactivé

Le point de terminaison webhook vers **{{webhook.host}}** de {{establishment.name}} a été désactivé automatiquement après {{webhook.failures}} livraisons consécutives en échec. Les événements suivants ne sont plus envoyés.

Vérifiez le service qui reçoit les webhooks, puis réactivez le point de terminaison dans Configuration › Intégrations : l'historique des tentatives montre la réponse de chaque envoi.
