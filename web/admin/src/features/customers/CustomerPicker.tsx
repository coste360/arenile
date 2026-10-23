import { Button } from '@arenile/shared/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@arenile/shared/ui/command';
import { Popover, PopoverContent, PopoverTrigger } from '@arenile/shared/ui/popover';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Spinner } from '@arenile/shared/ui/spinner';
import { ChevronsUpDownIcon, UserRoundIcon, UsersRoundIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCustomerSearch, type Customer } from './api';
import { customerContact, duplicateTerm, useDebounced } from './lib';

export interface CustomerPickerProps {
  establishmentId: string;
  value: Customer | null;
  onChange: (customer: Customer | null) => void;
  id?: string;
  invalid?: boolean;
  disabled?: boolean;
}

/** Customer search combobox (shadcn Popover + Command, server-side search by name/phone/email). */
export function CustomerPicker({
  establishmentId,
  value,
  onChange,
  id,
  invalid,
  disabled,
}: CustomerPickerProps) {
  const { t } = useTranslation('customers');
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState('');
  const term = useDebounced(q);
  const search = useCustomerSearch(establishmentId, term);
  const items = term.trim().length >= 2 ? (search.data ?? []) : [];
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-invalid={invalid || undefined}
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          {value ? (
            <span className="flex min-w-0 items-center gap-2">
              <UserRoundIcon aria-hidden="true" />
              <span className="truncate">{value.display_name}</span>
              <span className="truncate text-muted-foreground">{customerContact(value)}</span>
            </span>
          ) : (
            <span className="text-muted-foreground">{t('picker.placeholder')}</span>
          )}
          <ChevronsUpDownIcon aria-hidden="true" className="opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput
            value={q}
            onValueChange={setQ}
            placeholder={t('picker.search')}
            aria-label={t('picker.search')}
          />
          <CommandList>
            {search.isFetching && (
              <div className="flex justify-center p-2" role="status">
                <Spinner />
                <span className="sr-only">{t('picker.searching')}</span>
              </div>
            )}
            <CommandEmpty>
              {term.trim().length < 2 ? t('picker.typeMore') : t('picker.none')}
            </CommandEmpty>
            {items.length > 0 && (
              <CommandGroup>
                {items.map((c) => (
                  <CommandItem
                    key={c.id}
                    value={c.id}
                    onSelect={() => {
                      onChange(c);
                      setOpen(false);
                    }}
                  >
                    <span className="truncate">{c.display_name}</span>
                    <span className="ml-auto truncate text-xs text-muted-foreground">
                      {customerContact(c)}
                    </span>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

export interface DuplicateHintProps {
  establishmentId: string;
  candidate: { last_name?: string; email?: string; phone?: string };
  onUse: (customer: Customer) => void;
}

/** "Possibly already registered" hint while a new customer is typed inline (dedupe). */
export function DuplicateHint({ establishmentId, candidate, onUse }: DuplicateHintProps) {
  const { t } = useTranslation('customers');
  const term = useDebounced(duplicateTerm(candidate), 400);
  const search = useCustomerSearch(establishmentId, term, 3);
  const matches = term ? (search.data ?? []) : [];
  if (matches.length === 0) return null;
  return (
    <Alert aria-live="polite">
      <UsersRoundIcon aria-hidden="true" />
      <AlertTitle>{t('duplicates.title')}</AlertTitle>
      <AlertDescription>
        <ul className="w-full space-y-1">
          {matches.map((c) => (
            <li key={c.id} className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate">
                {c.display_name}
                <span className="text-muted-foreground"> · {customerContact(c)}</span>
              </span>
              <Button size="sm" variant="outline" onClick={() => onUse(c)}>
                {t('duplicates.use')}
              </Button>
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}
