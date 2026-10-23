import { CheckIcon, ChevronsUpDownIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@arenile/shared/ui/command';
import { Popover, PopoverContent, PopoverTrigger } from '@arenile/shared/ui/popover';
import { cn } from '@arenile/shared/lib/utils';

import type { Option } from './OptionSelect';

export interface MultiSelectProps {
  value: string[];
  onChange: (v: string[]) => void;
  options: readonly Option[];
  /** Shown when nothing is chosen (default "Tutti"). */
  placeholder?: string;
  disabled?: boolean;
  className?: string;
  id?: string;
  'aria-label'?: string;
  'aria-labelledby'?: string;
  'aria-describedby'?: string;
  'aria-invalid'?: boolean;
}

/**
 * Multi-select on Popover + Command (search above 6 options, check marks,
 * clear). The trigger shows the chosen items as badges; the value keeps the
 * options' order, independent of click order.
 */
export function MultiSelect({
  value,
  onChange,
  options,
  placeholder,
  disabled,
  className,
  ...aria
}: MultiSelectProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const selected = options.filter((o) => value.includes(o.value));
  const toggle = (v: string) => {
    const next = value.includes(v) ? value.filter((x) => x !== v) : [...value, v];
    onChange(options.map((o) => o.value).filter((o) => next.includes(o)));
  };
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn('h-auto min-h-9 w-full justify-between px-3 py-1.5 font-normal', className)}
          {...aria}
        >
          <span className="flex min-w-0 flex-wrap gap-1">
            {selected.length === 0 && (
              <span className="text-muted-foreground">{placeholder ?? t('ui.multi.all')}</span>
            )}
            {selected.slice(0, 4).map((o) => (
              <Badge key={o.value} variant="secondary">
                {o.label}
              </Badge>
            ))}
            {selected.length > 4 && (
              <Badge variant="outline">{t('ui.multi.more', { count: selected.length - 4 })}</Badge>
            )}
          </span>
          <ChevronsUpDownIcon aria-hidden="true" className="text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) min-w-56 p-0" align="start">
        <Command>
          {options.length > 6 && <CommandInput placeholder={t('ui.multi.search')} />}
          <CommandList>
            <CommandEmpty>{t('ui.noResults')}</CommandEmpty>
            <CommandGroup>
              {options.map((o) => {
                const on = value.includes(o.value);
                return (
                  <CommandItem
                    key={o.value}
                    value={`${typeof o.label === 'string' ? o.label : ''} ${o.value}`}
                    onSelect={() => toggle(o.value)}
                    aria-selected={on}
                  >
                    <span
                      aria-hidden="true"
                      className={cn(
                        'flex size-4 items-center justify-center rounded-sm border',
                        on ? 'border-primary bg-primary text-primary-foreground' : 'opacity-60',
                      )}
                    >
                      {on && <CheckIcon className="size-3" />}
                    </span>
                    {o.label}
                  </CommandItem>
                );
              })}
            </CommandGroup>
            {value.length > 0 && (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem onSelect={() => onChange([])} className="justify-center">
                    <XIcon aria-hidden="true" />
                    {t('ui.multi.clear')}
                  </CommandItem>
                </CommandGroup>
              </>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
