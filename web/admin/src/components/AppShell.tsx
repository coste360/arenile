import { useLogout, useMe } from '@arenile/shared/api';
import { Button, cn } from '@arenile/shared/ui';
import { Link, Outlet, useNavigate } from '@tanstack/react-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { EstablishmentSwitcher, useCurrentEstablishment, useIsOwner } from '../establishment';
import { CONFIG_NAV_ITEMS, NAV_ITEMS } from '../nav';
import { OfflineIndicator } from './OfflineIndicator';

/** Panel layout with the navigation of spec 15.1. */
export function AppShell() {
  const { t } = useTranslation();
  const me = useMe();
  const logout = useLogout();
  const navigate = useNavigate();
  const [menuOpen, setMenuOpen] = useState(false);

  const onLogout = () => {
    logout.mutate(undefined, {
      onSettled: () => {
        void navigate({ to: '/login', search: { redirect: undefined } });
      },
    });
  };

  const userName = me.data?.user?.name || me.data?.api_key?.name || '';
  const current = useCurrentEstablishment();
  const isOwner = useIsOwner();
  const linkClass =
    'block rounded-md px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100';
  const activeLinkProps = {
    className:
      'bg-[var(--ar-primary)] !text-[var(--ar-primary-foreground)] hover:!bg-[var(--ar-primary)]',
    'aria-current': 'page' as const,
  };

  return (
    <div className="min-h-screen bg-slate-50">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-50 focus:rounded focus:bg-white focus:px-3 focus:py-2"
      >
        {t('admin.skipToContent')}
      </a>
      <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-slate-200 bg-white px-4">
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          aria-expanded={menuOpen}
          aria-controls="main-nav"
          aria-label={menuOpen ? t('nav.closeMenu') : t('nav.openMenu')}
          onClick={() => setMenuOpen((o) => !o)}
        >
          <span aria-hidden="true" className="text-xl leading-none">
            ☰
          </span>
        </Button>
        <span className="font-semibold text-[var(--ar-primary-on-surface)]">
          {t('common.appName')}
        </span>
        <div className="ml-auto flex items-center gap-3">
          <EstablishmentSwitcher />
          <OfflineIndicator />
          {userName && (
            <span className="hidden text-sm text-slate-700 sm:inline" title={t('admin.user')}>
              {userName}
            </span>
          )}
          <Button variant="outline" size="sm" onClick={onLogout} disabled={logout.isPending}>
            {t('auth.logout')}
          </Button>
        </div>
      </header>
      {me.data?.totp_enrolment_required && (
        <div
          role="alert"
          className="border-b border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-900"
        >
          {t('admin.totpEnrolmentRequired')}
        </div>
      )}
      <div className="md:flex">
        <div
          id="main-nav"
          className={cn(
            'border-b border-slate-200 bg-white md:sticky md:top-14 md:block md:h-[calc(100vh-3.5rem)] md:w-56 md:shrink-0 md:overflow-y-auto md:border-b-0 md:border-r',
            menuOpen ? 'block' : 'hidden',
          )}
        >
          <nav aria-label={t('nav.main')}>
            <ul className="space-y-0.5 p-2">
              {NAV_ITEMS.map((item) => (
                <li key={item.to}>
                  <Link
                    to={item.to}
                    onClick={() => setMenuOpen(false)}
                    className={linkClass}
                    activeProps={activeLinkProps}
                  >
                    {t(item.labelKey)}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
          {current.id && (
            <nav aria-label={t('nav.configuration')} className="border-t border-slate-200">
              <p className="px-5 pb-1 pt-3 text-xs font-semibold uppercase tracking-wide text-slate-500">
                {current.establishment?.name ?? t('nav.configuration')}
              </p>
              <ul className="space-y-0.5 p-2 pt-0">
                {CONFIG_NAV_ITEMS.filter((item) => !('ownerOnly' in item) || isOwner).map(
                  (item) => (
                    <li key={item.to}>
                      <Link
                        to={item.to}
                        params={{ establishmentId: current.id as string }}
                        onClick={() => setMenuOpen(false)}
                        className={linkClass}
                        activeProps={activeLinkProps}
                      >
                        {t(item.labelKey)}
                      </Link>
                    </li>
                  ),
                )}
              </ul>
            </nav>
          )}
        </div>
        <main id="main" tabIndex={-1} className="min-w-0 flex-1 p-4 md:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
