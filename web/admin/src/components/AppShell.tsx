import { useMe } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Separator } from '@arenile/shared/ui/separator';
import { SidebarInset, SidebarProvider, SidebarTrigger } from '@arenile/shared/ui/sidebar';
import { Outlet } from '@tanstack/react-router';
import { ShieldAlertIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { AppBreadcrumb } from './AppBreadcrumb';
import { AppSidebar } from './AppSidebar';
import { CommandMenu } from './CommandMenu';
import { OfflineIndicator } from './OfflineIndicator';

/** Sidebar state persisted by shadcn's SidebarProvider in the `sidebar_state` cookie. */
function sidebarDefaultOpen(): boolean {
  if (typeof document === 'undefined') return true;
  return !/(?:^|;\s*)sidebar_state=false/.test(document.cookie);
}

/** Panel layout: shadcn Sidebar + top bar (breadcrumb, command palette, connection). */
export function AppShell() {
  const { t } = useTranslation();
  const me = useMe();

  return (
    <SidebarProvider defaultOpen={sidebarDefaultOpen()}>
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-md focus:bg-background focus:px-3 focus:py-2 focus:shadow"
      >
        {t('admin.skipToContent')}
      </a>
      <AppSidebar />
      <SidebarInset className="min-w-0">
        <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 rounded-t-xl border-b bg-background px-4 print:hidden">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 data-[orientation=vertical]:h-4" />
          <AppBreadcrumb />
          <div className="ml-auto flex items-center gap-2">
            <CommandMenu />
            <OfflineIndicator />
          </div>
        </header>
        {me.data?.totp_enrolment_required && (
          <div className="px-4 pt-4 md:px-6">
            <Alert>
              <ShieldAlertIcon aria-hidden="true" />
              <AlertDescription>{t('admin.totpEnrolmentRequired')}</AlertDescription>
            </Alert>
          </div>
        )}
        <div id="main" tabIndex={-1} className="flex-1 p-4 outline-none md:p-6 lg:p-8">
          <div className="mx-auto w-full max-w-7xl">
            <Outlet />
          </div>
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
