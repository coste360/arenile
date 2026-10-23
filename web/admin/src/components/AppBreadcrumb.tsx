import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@arenile/shared/ui/breadcrumb';
import { useMatchRoute } from '@tanstack/react-router';
import { Fragment } from 'react';
import { useTranslation } from 'react-i18next';
import { useCurrentEstablishment } from '../establishment';
import {
  BREADCRUMB_ONLY_ITEMS,
  CONFIG_NAV_ITEMS,
  ESTABLISHMENTS_NAV_ITEM,
  NAV_ITEMS,
} from '../nav';

/** Where am I: group › (establishment ›) page, derived from the matched route. */
export function AppBreadcrumb() {
  const { t } = useTranslation();
  const matchRoute = useMatchRoute();
  const current = useCurrentEstablishment();

  let trail: string[] = [];
  const op = NAV_ITEMS.find((i) => matchRoute({ to: i.to, fuzzy: true } as never));
  if (op) {
    trail = [
      t('nav.operations'),
      ...(current.establishment ? [current.establishment.name] : []),
      t(op.labelKey),
    ];
  } else if (matchRoute({ to: ESTABLISHMENTS_NAV_ITEM.to, fuzzy: true })) {
    trail = [t('nav.configGroup'), t(ESTABLISHMENTS_NAV_ITEM.labelKey)];
  } else if (matchRoute({ to: '/establishments/new' })) {
    trail = [t('nav.configGroup'), t('admin.switcherNew')];
  } else {
    // Longest path first so /pricing/simulator does not stop at /pricing and
    // /config/devices/lan gets its own label instead of "Dispositivi".
    const cfg = [...CONFIG_NAV_ITEMS, ...BREADCRUMB_ONLY_ITEMS]
      .sort((a, b) => b.to.length - a.to.length)
      .find((i) => matchRoute({ to: i.to, fuzzy: true } as never));
    if (cfg) {
      trail = [
        t('nav.configGroup'),
        ...(current.establishment ? [current.establishment.name] : []),
        t(cfg.labelKey),
      ];
    }
  }
  if (trail.length === 0) return null;

  return (
    <Breadcrumb>
      <BreadcrumbList>
        {trail.map((label, i) => {
          const last = i === trail.length - 1;
          return (
            <Fragment key={`${i}-${label}`}>
              <BreadcrumbItem className={last ? undefined : 'hidden md:inline-flex'}>
                {last ? <BreadcrumbPage>{label}</BreadcrumbPage> : label}
              </BreadcrumbItem>
              {!last && <BreadcrumbSeparator className="hidden md:block" />}
            </Fragment>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
