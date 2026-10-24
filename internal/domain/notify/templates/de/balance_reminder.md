---
subject: Erinnerung: Restbetrag der Buchung {codice} – {stabilimento}
preheader: Offener Restbetrag: {{booking.balance}}.
opening: Hallo {nome}, wir möchten Sie daran erinnern, dass für die Buchung {codice} bei {stabilimento} noch der Restbetrag offen ist.
closing: Sie können online auf der Buchungsseite oder an der Kasse bezahlen. Falls Sie bereits bezahlt haben, betrachten Sie diese Nachricht bitte als gegenstandslos.
signature:
---
# Erinnerung an den Restbetrag

{{> opening}}

{{> balance_notice}}

{{> summary}}

{{> manage_button}}

{{link.expires}}

{{> closing}}

{{> signature}}
