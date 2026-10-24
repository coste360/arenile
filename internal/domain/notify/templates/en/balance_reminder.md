---
subject: Reminder: balance of booking {codice} – {stabilimento}
preheader: Balance due: {{booking.balance}}.
opening: Hi {nome}, this is a reminder that the balance of booking {codice} at {stabilimento} is still to be paid.
closing: You can pay online from the booking page or at the desk. If you have already paid, please ignore this message.
signature:
---
# Balance reminder

{{> opening}}

{{> balance_notice}}

{{> summary}}

{{> manage_button}}

{{link.expires}}

{{> closing}}

{{> signature}}
