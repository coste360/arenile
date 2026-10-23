---
subject: Prenotazione {{booking.code}} modificata – {{establishment.name}}
preheader: Ecco il riepilogo aggiornato.
---
# La tua prenotazione è stata modificata

Ciao {{customer.first_name}}, abbiamo aggiornato la prenotazione **{{booking.code}}**. Ecco il nuovo riepilogo.

{{> summary}}

{{> qr}}

{{> manage_button}}

{{link.expires}}

{{> ics_note}}
