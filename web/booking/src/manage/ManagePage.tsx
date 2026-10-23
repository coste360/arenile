import { useTranslation } from 'react-i18next';
import { PUBLIC_NS } from '../i18n';
import type { BookingLocation } from '../location';

export interface ManagePageProps {
  location: BookingLocation;
}

/**
 * Customer manage area at `/b/{slug}/m/{token}` (spec 12.4), lazy-loaded
 * from main.tsx: details and QR, pay balance, modify / cancel within the
 * policy with refund or credit preview (dry runs), release days for season
 * pass holders, and "send me a new link" (email + booking code) when the
 * link is invalid. Owner: booking app builder.
 */
export function ManagePage({ location }: ManagePageProps) {
  const { t } = useTranslation(PUBLIC_NS);
  return (
    <main className="mx-auto max-w-xl px-4 py-12 text-center">
      <h1 className="text-2xl font-semibold">{t('manage.title')}</h1>
      {!location.manageToken && (
        <p role="alert" className="mt-4 text-muted-foreground">
          {t('manage.linkInvalid')}
        </p>
      )}
    </main>
  );
}

export default ManagePage;
