---
subject: Buchung {{booking.code}} geändert – {{establishment.name}}
preheader: Hier ist die aktualisierte Übersicht.
---
# Ihre Buchung wurde geändert

Hallo {{customer.first_name}}, wir haben die Buchung **{{booking.code}}** aktualisiert. Hier ist die neue Übersicht.

{{> summary}}

{{> qr}}

{{> manage_button}}

{{link.expires}}

{{> ics_note}}
