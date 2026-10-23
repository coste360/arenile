import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { SidebarMenu, SidebarMenuButton, SidebarMenuItem } from '@arenile/shared/ui/sidebar';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { useNavigate, useParams } from '@tanstack/react-router';
import { ChevronsUpDownIcon, PlusIcon, UmbrellaIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment, useIsOwner } from './context';

/**
 * Establishment switcher in the sidebar header (spec 28 M1,
 * multi-establishment): keeps the current section when switching.
 */
export function EstablishmentSwitcher() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as {
    establishmentId?: string;
    mapId?: string;
    bookingId?: string;
    customerId?: string;
  };
  const isOwner = useIsOwner();
  const current = useCurrentEstablishment();

  const onSelect = (value: string) => {
    if (params.mapId) {
      // A map document belongs to one establishment: go back to the maps list.
      void navigate({ to: '/e/$establishmentId/maps', params: { establishmentId: value } });
    } else if (params.bookingId) {
      // Bookings and customers belong to one establishment: back to the list.
      void navigate({ to: '/e/$establishmentId/prenotazioni', params: { establishmentId: value } });
    } else if (params.customerId) {
      void navigate({ to: '/e/$establishmentId/clienti', params: { establishmentId: value } });
    } else if (params.establishmentId) {
      // Same section, other establishment.
      void navigate({ to: '.', params: { establishmentId: value } as never });
    } else {
      void navigate({
        to: '/e/$establishmentId/config/establishment',
        params: { establishmentId: value },
      });
    }
  };

  const name = current.establishment?.name;
  const subtitle = current.isLoading
    ? t('common.loading')
    : current.items.length === 0
      ? t('admin.switcher.none')
      : (name ?? t('admin.switcher.placeholder'));

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
              disabled={current.isLoading}
            >
              <span className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                <UmbrellaIcon aria-hidden="true" className="size-4" />
              </span>
              <span className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-semibold">{t('common.appName')}</span>
                {/* Visible text is part of the name: "Arenile Stabilimento: Lido Demo". */}
                <span className="sr-only">{t('admin.switcher.label')}: </span>
                {current.isLoading ? (
                  <Skeleton className="mt-1 h-3 w-24" />
                ) : (
                  <span className="truncate text-xs text-muted-foreground">{subtitle}</span>
                )}
              </span>
              <ChevronsUpDownIcon aria-hidden="true" className="ml-auto" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
            align="start"
            sideOffset={4}
          >
            <DropdownMenuLabel className="text-xs text-muted-foreground">
              {t('admin.switcher.label')}
            </DropdownMenuLabel>
            <DropdownMenuRadioGroup value={current.id ?? ''} onValueChange={onSelect}>
              {current.items.map((e) => (
                <DropdownMenuRadioItem key={e.id} value={e.id}>
                  <span className="truncate">{e.name}</span>
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
            {isOwner && (
              <>
                {current.items.length > 0 && <DropdownMenuSeparator />}
                <DropdownMenuItem onSelect={() => void navigate({ to: '/establishments/new' })}>
                  <PlusIcon aria-hidden="true" />
                  {t('admin.switcherNew')}
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
