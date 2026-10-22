import { useEstablishments, type Establishment } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { StatusBadge } from '@arenile/shared/ui/patterns';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { ExternalLinkIcon } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { problemMessageKey } from '@arenile/shared/api';

function Rows({ items }: { items: Establishment[] }) {
  const { t } = useTranslation();
  return (
    <>
      {items.map((e) => (
        <TableRow key={e.id}>
          <TableHead scope="row" className="font-medium text-foreground">
            {e.name}
          </TableHead>
          <TableCell>
            <a
              href={`/b/${encodeURIComponent(e.slug)}`}
              className="inline-flex items-center gap-1 text-primary-on-surface underline-offset-4 hover:underline"
              aria-label={t('admin.establishments.openPublicPage', { name: e.name })}
            >
              /b/{e.slug}
              <ExternalLinkIcon aria-hidden="true" className="size-3.5" />
            </a>
          </TableCell>
          <TableCell>
            {t(`common.languages.${e.default_locale}`, { defaultValue: e.default_locale })}
          </TableCell>
          <TableCell>
            <StatusBadge tone={e.booking_enabled ? 'success' : 'neutral'}>
              {e.booking_enabled
                ? t('admin.establishments.bookingOn')
                : t('admin.establishments.bookingOff')}
            </StatusBadge>
          </TableCell>
        </TableRow>
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
      <TableRow>
        <TableCell colSpan={4}>
          <span className="sr-only">{t('common.loading')}</span>
          <Skeleton aria-hidden="true" className="h-4 w-48" />
        </TableCell>
      </TableRow>
    );
  }
  if (query.isError) {
    return (
      <TableRow>
        <TableCell colSpan={4}>
          <Alert variant="destructive" className="flex items-center justify-between gap-3">
            <AlertDescription>{t(problemMessageKey(query.error))}</AlertDescription>
            <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
              {t('common.retry')}
            </Button>
          </Alert>
        </TableCell>
      </TableRow>
    );
  }
  const { items, next_cursor: next } = query.data;
  return (
    <>
      {items.length === 0 && !cursor && (
        <TableRow>
          <TableCell colSpan={4} className="py-10 text-center text-muted-foreground">
            {t('admin.establishments.empty')}
          </TableCell>
        </TableRow>
      )}
      <Rows items={items} />
      {isLast && next && (
        <TableRow className="hover:bg-transparent">
          <TableCell colSpan={4} className="text-center">
            <Button variant="outline" size="sm" onClick={() => onMore(next)}>
              {t('admin.establishments.loadMore')}
            </Button>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

/** Establishments accessible to the current user (M0 endpoint, cursor pagination). */
export function EstablishmentsPage() {
  const { t } = useTranslation();
  const [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  return (
    <section aria-labelledby="establishments-title" className="space-y-3">
      <h2 id="establishments-title" className="text-lg font-semibold">
        {t('admin.establishments.title')}
      </h2>
      <div className="overflow-hidden rounded-lg border bg-card">
        <Table className="min-w-[32rem]">
          <TableHeader className="bg-muted/50">
            <TableRow className="hover:bg-transparent">
              <TableHead scope="col">{t('admin.establishments.name')}</TableHead>
              <TableHead scope="col">{t('admin.establishments.slug')}</TableHead>
              <TableHead scope="col">{t('admin.establishments.locale')}</TableHead>
              <TableHead scope="col">{t('admin.establishments.booking')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {cursors.map((c, i) => (
              <PageChunk
                key={c ?? 'first'}
                cursor={c}
                isLast={i === cursors.length - 1}
                onMore={(next) => setCursors((list) => [...list, next])}
              />
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}
