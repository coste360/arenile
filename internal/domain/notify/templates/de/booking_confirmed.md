---
subject: Buchung {{booking.code}} bestätigt – {{establishment.name}}
preheader: Wir freuen uns auf Sie {{booking.period}}.
---
# Ihre Buchung ist bestätigt

Hallo {{customer.first_name}}, vielen Dank, dass Sie sich für **{{establishment.name}}** entschieden haben. Hier ist die Übersicht Ihrer Buchung.

{{> summary}}

{{> qr}}

{{> manage_button}}

Auf der Buchungsseite sehen Sie alle Details, können den Restbetrag zahlen und die Buchung im Rahmen der Bedingungen ändern oder stornieren. {{link.expires}}

{{> ics_note}}

{{> policy}}

Bis bald am Strand!
