---
subject: Rappel : solde de la réservation {codice} – {stabilimento}
preheader: Solde à payer : {{booking.balance}}.
opening: Bonjour {nome}, nous vous rappelons que le solde de la réservation {codice} chez {stabilimento} reste à payer.
closing: Vous pouvez payer en ligne depuis la page de gestion ou à la caisse. Si vous avez déjà payé, merci d'ignorer ce message.
signature:
---
# Rappel du solde

{{> opening}}

{{> balance_notice}}

{{> summary}}

{{> manage_button}}

{{link.expires}}

{{> closing}}

{{> signature}}
