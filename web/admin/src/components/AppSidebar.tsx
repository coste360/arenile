import { Badge } from '@arenile/shared/ui/badge';
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@arenile/shared/ui/sidebar';
import { Link, useMatchRoute } from '@tanstack/react-router';
import { useTranslation } from 'react-i18next';
import { EstablishmentSwitcher, useCurrentEstablishment, useIsOwner } from '../establishment';
import { CONFIG_NAV_ITEMS, ESTABLISHMENTS_NAV_ITEM, NAV_ITEMS } from '../nav';
import { NavUser } from './NavUser';

/** shadcn Sidebar (collapsible to icons) with the panel navigation of spec 15.1. */
export function AppSidebar() {
  const { t } = useTranslation();
  const matchRoute = useMatchRoute();
  const { setOpenMobile } = useSidebar();
  const current = useCurrentEstablishment();
  const isOwner = useIsOwner();
  const close = () => setOpenMobile(false);

  return (
    <Sidebar collapsible="icon" variant="inset">
      <SidebarHeader>
        <EstablishmentSwitcher />
      </SidebarHeader>
      <SidebarContent>
        <nav aria-label={t('nav.main')}>
          <SidebarGroup>
            <SidebarGroupLabel>{t('nav.operations')}</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {NAV_ITEMS.map((item) => {
                  const label = t(item.labelKey);
                  const active = !!matchRoute({ to: item.to, fuzzy: true });
                  return (
                    <SidebarMenuItem key={item.to}>
                      <SidebarMenuButton
                        asChild
                        isActive={active}
                        tooltip={item.available ? label : `${label} · ${t('nav.soon')}`}
                        className="aria-disabled:pointer-events-none aria-disabled:opacity-60"
                      >
                        <Link
                          to={item.to}
                          onClick={close}
                          aria-current={active ? 'page' : undefined}
                          aria-disabled={item.available ? undefined : true}
                          tabIndex={item.available ? undefined : -1}
                        >
                          <item.icon aria-hidden="true" />
                          <span>{label}</span>
                        </Link>
                      </SidebarMenuButton>
                      {!item.available && (
                        <SidebarMenuBadge>
                          <Badge
                            variant="outline"
                            className="border-highlight/60 bg-highlight/15 px-1.5 text-[10px] font-medium text-muted-foreground"
                          >
                            {t('nav.soon')}
                          </Badge>
                        </SidebarMenuBadge>
                      )}
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
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
                  CONFIG_NAV_ITEMS.filter((item) => !('ownerOnly' in item) || isOwner).map(
                    (item) => (
                      <ConfigLink
                        key={item.to}
                        to={item.to}
                        params={{ establishmentId: current.id as string }}
                        label={t(item.labelKey)}
                        icon={item.icon}
                        onClick={close}
                      />
                    ),
                  )}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </nav>
      </SidebarContent>
      <SidebarFooter>
        <NavUser />
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
  return (
    <SidebarMenuItem>
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
