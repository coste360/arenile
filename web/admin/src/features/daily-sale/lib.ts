/** Daily number label shown on the map and on the receipt ("#037"). */
export function ticketNumberLabel(n: number): string {
  return `#${String(n).padStart(3, '0')}`;
}

/** Map label of a sold spot: the holder name when given, else the number. */
export function ticketMapLabel(t: { ticket_number: number; holder_name?: string }): string {
  return t.holder_name?.trim() ? t.holder_name.trim() : ticketNumberLabel(t.ticket_number);
}
