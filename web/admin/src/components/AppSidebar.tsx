import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@arenile/shared/ui/collapsible';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@arenile/shared/ui/sidebar';
import { Coste360Signature } from '@arenile/shared/ui/patterns';
import { Link, useMatchRoute } from '@tanstack/react-router';
import { ChevronDownIcon } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { EstablishmentSwitcher, useCurrentEstablishment, useIsOwner } from '../establishment';
import { useIsDesktopEdition } from '../lib/meta';
import { CONFIG_NAV_ITEMS, ESTABLISHMENTS_NAV_ITEM, NAV_ITEMS } from '../nav';
import { NavUser } from './NavUser';

/**
 * shadcn Sidebar (collapsible to icons) with the panel navigation of spec 15.1.
 * Sections not delivered yet sit in one closed "In arrivo" group, so the
 * working configuration items stay above the fold on a tablet.
 */
export function AppSidebar() {
  const { t } = useTranslation();
  const matchRoute = useMatchRoute();
  const { setOpenMobile } = useSidebar();
  const current = useCurrentEstablishment();
  const isOwner = useIsOwner();
  const isDesktop = useIsDesktopEdition();
  const close = () => setOpenMobile(false);
  const available = NAV_ITEMS.filter((item) => item.available);
  const upcoming = NAV_ITEMS.filter((item) => !item.available);

  const operationLink = (item: (typeof NAV_ITEMS)[number]) => {
    const label = t(item.labelKey);
    // Operational sections are establishment-scoped; without an establishment
    // the legacy path redirects to the establishments list.
    const params = current.id ? { establishmentId: current.id } : undefined;
    const to = params ? item.to : `/${item.section}`;
    const active = !!matchRoute({ to, params, fuzzy: true } as never);
    return (
      <SidebarMenuItem key={item.to}>
        <SidebarMenuButton
          asChild
          isActive={active}
          tooltip={item.available ? label : `${label} · ${t('common.inArrivo')}`}
          className="aria-disabled:pointer-events-none aria-disabled:opacity-60"
        >
          <Link
            to={to as never}
            params={params as never}
            onClick={close}
            aria-current={active ? 'page' : undefined}
            aria-disabled={item.available ? undefined : true}
            tabIndex={item.available ? undefined : -1}
          >
            <item.icon aria-hidden="true" />
            <span>{label}</span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    );
  };

  return (
    // A complementary landmark: the switcher, the user menu and the signature
    // live outside the two <nav>s and would otherwise sit outside any landmark.
    <Sidebar
      collapsible="icon"
      variant="inset"
      className="print:hidden"
      role="complementary"
      aria-label={t('ui.sidebar')}
    >
      <SidebarHeader>
        <EstablishmentSwitcher />
      </SidebarHeader>
      <SidebarContent>
        <nav aria-label={t('nav.main')}>
          <SidebarGroup>
            <SidebarGroupLabel>{t('nav.operations')}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>{available.map(operationLink)}</SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          {upcoming.length > 0 && (
            <Collapsible className="group/upcoming">
              <SidebarGroup>
                <SidebarGroupLabel asChild>
                  <CollapsibleTrigger>
                    {t('common.inArrivo')}
                    <span className="ml-1 tabular-nums">({upcoming.length})</span>
                    <ChevronDownIcon
                      aria-hidden="true"
                      className="ml-auto transition-transform group-data-[state=open]/upcoming:rotate-180"
                    />
                  </CollapsibleTrigger>
                </SidebarGroupLabel>
                <CollapsibleContent>
                  <SidebarGroupContent>
                    <SidebarMenu>{upcoming.map(operationLink)}</SidebarMenu>
                  </SidebarGroupContent>
                </CollapsibleContent>
              </SidebarGroup>
            </Collapsible>
          )}
        </nav>
        <nav aria-label={t('nav.configuration')}>
          <SidebarGroup>
            <SidebarGroupLabel className="gap-1">
              <span>{t('nav.configGroup')}</span>
              {current.establishment && (
                <>
                  <span aria-hidden="true">·</span>
                  <span className="truncate font-normal">{current.establishment.name}</span>
                </>
              )}
            </SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <ConfigLink
                  to={ESTABLISHMENTS_NAV_ITEM.to}
                  label={t(ESTABLISHMENTS_NAV_ITEM.labelKey)}
                  icon={ESTABLISHMENTS_NAV_ITEM.icon}
                  onClick={close}
                />
                {current.id &&
                  CONFIG_NAV_ITEMS.filter(
                    (item) =>
                      (!('ownerOnly' in item) || isOwner) &&
                      (!('desktopOnly' in item) || isDesktop),
                  ).map((item) => (
                    <ConfigLink
                      key={item.to}
                      to={item.to}
                      params={{ establishmentId: current.id as string }}
                      label={t(item.labelKey)}
                      icon={item.icon}
                      onClick={close}
                    />
                  ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </nav>
      </SidebarContent>
      <SidebarFooter>
        <NavUser />
        <Coste360Signature className="group-data-[collapsible=icon]:hidden" />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

function ConfigLink({
  to,
  params,
  label,
  icon: Icon,
  onClick,
}: {
  to: string;
  params?: { establishmentId: string };
  label: string;
  icon: (typeof CONFIG_NAV_ITEMS)[number]['icon'];
  onClick: () => void;
}) {
  const matchRoute = useMatchRoute();
  const active = !!matchRoute({ to, params, fuzzy: true } as never);
  const ref = useRef<HTMLLIElement>(null);
  // Keep the current page's item visible when the list is taller than the viewport.
  useEffect(() => {
    if (active) ref.current?.scrollIntoView?.({ block: 'nearest' });
  }, [active]);
  return (
    <SidebarMenuItem ref={ref}>
      <SidebarMenuButton asChild isActive={active} tooltip={label}>
        <Link
          to={to as never}
          params={params as never}
          onClick={onClick}
          aria-current={active ? 'page' : undefined}
        >
          <Icon aria-hidden="true" />
          <span>{label}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}
