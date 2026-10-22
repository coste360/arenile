import { useEstablishments, type Establishment } from '@arenile/shared/api';
import { Button, Card } from '@arenile/shared/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '../lib/problem';

function Rows({ items }: { items: Establishment[] }) {
  const { t } = useTranslation();
  return (
    <>
      {items.map((e) => (
        <tr key={e.id} className="border-t border-slate-100">
          <th scope="row" className="px-4 py-3 text-left font-medium">
            {e.name}
          </th>
          <td className="px-4 py-3">
            <a
              href={`/b/${encodeURIComponent(e.slug)}`}
              className="text-[var(--ar-primary-on-surface)] underline underline-offset-2"
              aria-label={t('admin.establishments.openPublicPage', { name: e.name })}
            >
              /b/{e.slug}
            </a>
          </td>
          <td className="px-4 py-3">
            {t(`common.languages.${e.default_locale}`, { defaultValue: e.default_locale })}
          </td>
          <td className="px-4 py-3">
            {e.booking_enabled
              ? t('admin.establishments.bookingOn')
              : t('admin.establishments.bookingOff')}
          </td>
        </tr>
      ))}
    </>
  );
}

function PageChunk({
  cursor,
  isLast,
  onMore,
}: {
  cursor?: string;
  isLast: boolean;
  onMore: (c: string) => void;
}) {
  const { t } = useTranslation();
  const query = useEstablishments(cursor ? { cursor } : {});
  if (query.isPending) {
    return (
      <tr>
        <td colSpan={4} className="px-4 py-3 text-slate-600">
          {t('common.loading')}
        </td>
      </tr>
    );
  }
  if (query.isError) {
    return (
      <tr>
        <td colSpan={4} className="px-4 py-3">
          <span role="alert" className="text-red-700">
            {t(problemMessageKey(query.error))}
          </span>{' '}
          <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
            {t('common.retry')}
          </Button>
        </td>
      </tr>
    );
  }
  const { items, next_cursor: next } = query.data;
  return (
    <>
      {items.length === 0 && !cursor && (
        <tr>
          <td colSpan={4} className="px-4 py-6 text-center text-slate-600">
            {t('admin.establishments.empty')}
          </td>
        </tr>
      )}
      <Rows items={items} />
      {isLast && next && (
        <tr>
          <td colSpan={4} className="px-4 py-3 text-center">
            <Button variant="outline" size="sm" onClick={() => onMore(next)}>
              {t('admin.establishments.loadMore')}
            </Button>
          </td>
        </tr>
      )}
    </>
  );
}

/** Establishments accessible to the current user (M0 endpoint). */
export function EstablishmentsPage() {
  const { t } = useTranslation();
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  return (
    <section aria-labelledby="establishments-title" className="space-y-3">
      <h2 id="establishments-title" className="text-xl font-semibold">
        {t('admin.establishments.title')}
      </h2>
      <Card className="overflow-x-auto">
        <table className="w-full min-w-[32rem] text-sm">
          <thead className="bg-slate-50 text-left text-slate-600">
            <tr>
              <th scope="col" className="px-4 py-2 font-medium">
                {t('admin.establishments.name')}
              </th>
              <th scope="col" className="px-4 py-2 font-medium">
                {t('admin.establishments.slug')}
              </th>
              <th scope="col" className="px-4 py-2 font-medium">
                {t('admin.establishments.locale')}
              </th>
              <th scope="col" className="px-4 py-2 font-medium">
                {t('admin.establishments.booking')}
              </th>
            </tr>
          </thead>
          <tbody>
            {cursors.map((c, i) => (
              <PageChunk
                key={c ?? 'first'}
                cursor={c}
                isLast={i === cursors.length - 1}
                onMore={(next) => setCursors((list) => [...list, next])}
              />
            ))}
          </tbody>
        </table>
      </Card>
    </section>
  );
}
