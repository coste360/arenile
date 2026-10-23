import { Button } from '@arenile/shared/ui/button';
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@arenile/shared/ui/command';
import { Kbd, KbdGroup } from '@arenile/shared/ui/kbd';
import { useNavigate } from '@tanstack/react-router';
import { SearchIcon } from 'lucide-react';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useIsOwner } from '../establishment';
import { CONFIG_NAV_ITEMS, ESTABLISHMENTS_NAV_ITEM, NAV_ITEMS } from '../nav';

const isMac = typeof navigator !== 'undefined' && /Mac|iP(hone|ad|od)/.test(navigator.platform);

/** Command palette (Ctrl/Cmd+K) to jump to any available page. */
export function CommandMenu() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const current = useCurrentEstablishment();
  const isOwner = useIsOwner();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, []);

  const go = (to: string, params?: Record<string, string>) => {
    setOpen(false);
    void navigate({ to: to as never, params: params as never });
  };

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        onClick={() => setOpen(true)}
        className="w-9 justify-center px-0 text-muted-foreground sm:w-56 sm:justify-start sm:px-3"
        aria-keyshortcuts={isMac ? 'Meta+K' : 'Control+K'}
      >
        <SearchIcon aria-hidden="true" />
        <span className="sr-only sm:hidden">{t('nav.search')}</span>
        <span className="hidden flex-1 text-left font-normal sm:inline">
          {t('nav.searchPlaceholder')}
        </span>
        <KbdGroup className="hidden sm:inline-flex">
          <Kbd>{isMac ? '⌘' : 'Ctrl'}</Kbd>
          <Kbd>K</Kbd>
        </KbdGroup>
      </Button>
      <CommandDialog open={open} onOpenChange={setOpen}>
        <CommandInput placeholder={t('nav.searchPlaceholder')} />
        <CommandList>
          <CommandEmpty>{t('ui.noResults')}</CommandEmpty>
          <CommandGroup heading={t('nav.operations')}>
            {NAV_ITEMS.filter((i) => i.available).map((item) => (
              <CommandItem key={item.to} onSelect={() => go(item.to)}>
                <item.icon aria-hidden="true" />
                {t(item.labelKey)}
              </CommandItem>
            ))}
          </CommandGroup>
          <CommandGroup heading={t('nav.configGroup')}>
            <CommandItem onSelect={() => go(ESTABLISHMENTS_NAV_ITEM.to)}>
              <ESTABLISHMENTS_NAV_ITEM.icon aria-hidden="true" />
              {t(ESTABLISHMENTS_NAV_ITEM.labelKey)}
            </CommandItem>
            {current.id &&
              CONFIG_NAV_ITEMS.filter((i) => !('ownerOnly' in i) || isOwner).map((item) => (
                <CommandItem
                  key={item.to}
                  onSelect={() => go(item.to, { establishmentId: current.id as string })}
                >
                  <item.icon aria-hidden="true" />
                  {t(item.labelKey)}
                </CommandItem>
              ))}
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}
