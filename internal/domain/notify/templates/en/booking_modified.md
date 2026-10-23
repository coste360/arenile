---
subject: Booking {{booking.code}} changed – {{establishment.name}}
preheader: Here is the updated summary.
---
# Your booking has been changed

Hi {{customer.first_name}}, we updated booking **{{booking.code}}**. Here is the new summary.

{{> summary}}

{{> qr}}

{{> manage_button}}

{{link.expires}}

{{> ics_note}}
