---
subject: Réservation {{booking.code}} modifiée – {{establishment.name}}
preheader: Voici le récapitulatif mis à jour.
---
# Votre réservation a été modifiée

Bonjour {{customer.first_name}}, nous avons mis à jour la réservation **{{booking.code}}**. Voici le nouveau récapitulatif.

{{> summary}}

{{> qr}}

{{> manage_button}}

{{link.expires}}

{{> ics_note}}
