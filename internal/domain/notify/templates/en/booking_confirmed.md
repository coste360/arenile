---
subject: Booking {{booking.code}} confirmed – {{establishment.name}}
preheader: See you {{booking.period}}.
---
# Your booking is confirmed

Hi {{customer.first_name}}, thank you for choosing **{{establishment.name}}**. Here is the summary of your booking.

{{> summary}}

{{> qr}}

{{> manage_button}}

From the booking page you can see the details, pay the balance and, within the terms, change or cancel the booking. {{link.expires}}

{{> ics_note}}

{{> policy}}

See you on the beach!
