import { useEffect, useState } from 'react';
import type { Customer } from './api';

/** Debounced copy of a value (search boxes). */
export function useDebounced<T>(value: T, ms = 250): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

/** Secondary line of a customer in lists: phone, else email. */
export function customerContact(c: Pick<Customer, 'phone' | 'email'>): string {
  return c.phone || c.email || '';
}

/** Search term for duplicates: phone, else email, else surname (≥ 3 characters). */
export function duplicateTerm(c: { last_name?: string; email?: string; phone?: string }): string {
  const phone = (c.phone ?? '').replace(/[\s().-]/g, '');
  if (phone.length >= 6) return phone.replace(/^\+39/, '').replace(/^0039/, '');
  const email = (c.email ?? '').trim();
  if (email.length >= 3) return email;
  const last = (c.last_name ?? '').trim();
  return last.length >= 3 ? last : '';
}
