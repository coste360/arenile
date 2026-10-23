---
subject: Réservation {{booking.code}} confirmée – {{establishment.name}}
preheader: Rendez-vous {{booking.period}}.
---
# Votre réservation est confirmée

Bonjour {{customer.first_name}}, merci d'avoir choisi **{{establishment.name}}**. Voici le récapitulatif de votre réservation.

{{> summary}}

{{> qr}}

{{> manage_button}}

Depuis la page de gestion, vous pouvez consulter les détails, payer le solde et, dans le respect des conditions, modifier ou annuler la réservation. {{link.expires}}

{{> ics_note}}

{{> policy}}

À bientôt à la plage !
