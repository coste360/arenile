import { useLogout, useMe } from '@arenile/shared/api';
import { Avatar, AvatarFallback } from '@arenile/shared/ui/avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from '@arenile/shared/ui/sidebar';
import { ThemeRadioItems } from '@arenile/shared/ui/patterns';
import { useNavigate } from '@tanstack/react-router';
import { ChevronsUpDownIcon, InfoIcon, LogOutIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useOfflineLogout } from '../offline/useOfflineLogout';
import { UpdateCheckStatus } from '../features/updates/UpdateCheckStatus';
import { useSystemUpdate } from '../features/updates/api';

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?';
}

/** User menu in the sidebar footer: identity, theme choice and logout. */
export function NavUser() {
  const { t } = useTranslation();
  const { t: tu } = useTranslation('updates');
  const me = useMe();
  const logout = useLogout();
  const offline = useOfflineLogout();
  const navigate = useNavigate();
  const { isMobile } = useSidebar();
  const updates = useSystemUpdate();
  const name = me.data?.user?.name || me.data?.api_key?.name || '';
  const email = me.data?.user?.email ?? '';

  const onLogout = () => {
    // Offline mode (spec 21.2/23): send the queue first, then wipe the device copy.
    void offline.beforeLogout().finally(() =>
      logout.mutate(undefined, {
        onSettled: () => {
          void offline.clearAll();
          void navigate({ to: '/login', search: { redirect: undefined } });
        },
      }),
    );
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
            >
              {/* The visible name and email stay part of the accessible name (WCAG 2.5.3). */}
              <span className="sr-only">{t('admin.userMenu')}: </span>
              <Avatar className="size-8 rounded-lg">
                <AvatarFallback className="rounded-lg bg-primary/10 text-xs font-semibold text-primary-on-surface">
                  {initials(name)}
                </AvatarFallback>
              </Avatar>
              <span className="grid flex-1 text-left text-sm leading-tight">
                <span className="truncate font-medium">{name}</span>
                {email && <span className="truncate text-xs text-muted-foreground">{email}</span>}
              </span>
              <ChevronsUpDownIcon aria-hidden="true" className="ml-auto size-4" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-lg"
            side={isMobile ? 'bottom' : 'right'}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuLabel className="font-normal">
              <span className="block truncate font-medium">{name}</span>
              {email && (
                <span className="block truncate text-xs text-muted-foreground">{email}</span>
              )}
            </DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="text-xs text-muted-foreground">
              {t('ui.theme.label')}
            </DropdownMenuLabel>
            <ThemeRadioItems />
            <DropdownMenuSeparator />
            {/* Version and update check (ADR 0025): owners and managers only. */}
            {updates.data && (
              <DropdownMenuLabel className="font-normal">
                <UpdateCheckStatus />
              </DropdownMenuLabel>
            )}
            {/* Version and updates page, for everybody (the actions are for owners and managers). */}
            <DropdownMenuItem onSelect={() => void navigate({ to: '/informazioni' })}>
              <InfoIcon aria-hidden="true" />
              {tu('about.menu')}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onLogout} disabled={logout.isPending}>
              <LogOutIcon aria-hidden="true" />
              {t('auth.logout')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
