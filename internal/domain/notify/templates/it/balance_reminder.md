---
subject: Promemoria: saldo della prenotazione {codice} – {stabilimento}
preheader: Saldo da pagare: {{booking.balance}}.
opening: Ciao {nome}, ti ricordiamo che per la prenotazione {codice} presso {stabilimento} resta da pagare il saldo.
closing: Puoi pagare online dall'area di gestione o in cassa. Se hai già pagato, ignora questo messaggio.
signature:
---
# Promemoria del saldo

{{> opening}}

{{> balance_notice}}

{{> summary}}

{{> manage_button}}

{{link.expires}}

{{> closing}}

{{> signature}}
