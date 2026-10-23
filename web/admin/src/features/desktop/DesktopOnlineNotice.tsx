import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { PageHeader } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { CloudUploadIcon, MonitorIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';

/** The online sections that do not exist on the desktop edition. */
export type DesktopOnlineTopic = 'publicPage' | 'payments';

/**
 * Explains, in place of an online-only section, why the desktop edition
 * does not have it and how to move to an online instance (spec 29.1,
 * "Limiti voluti" and "Passaggio a un'istanza online"): the desktop backup
 * is the same SQLite file, restored with `arenile restore` on a VPS.
 */
export function DesktopOnlineNotice({ topic }: { topic: DesktopOnlineTopic }) {
  const { t } = useTranslation('desktop');
  return (
    <section aria-labelledby="page-title" className="mx-auto max-w-3xl space-y-6">
      <PageHeader title={t(`online.${topic}.title`)} />
      <Alert role="note">
        <MonitorIcon aria-hidden="true" />
        <AlertTitle>{t(`online.${topic}.notice`)}</AlertTitle>
        <AlertDescription>{t(`online.${topic}.why`)}</AlertDescription>
      </Alert>
      <Card>
        <CardHeader>
          <CardTitle>
            <h2 className="flex items-center gap-2 text-base">
              <CloudUploadIcon aria-hidden="true" className="size-4 text-muted-foreground" />
              {t('online.move.title')}
            </h2>
          </CardTitle>
          <CardDescription>{t('online.move.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <ol className="list-decimal space-y-3 pl-5 text-sm">
            <li>
              {t('online.move.step1')}{' '}
              <Button asChild variant="link" className="h-auto p-0">
                <Link to="/desktop">{t('online.move.step1Link')}</Link>
              </Button>
            </li>
            <li>{t('online.move.step2')}</li>
            <li className="space-y-2">
              <span>{t('online.move.step3')}</span>
              <code className="block rounded-md bg-muted px-3 py-2 font-mono text-xs break-all">
                arenile restore --in arenile-backup.db
              </code>
            </li>
            <li>{t('online.move.step4')}</li>
          </ol>
          <p className="mt-4 text-sm text-muted-foreground">{t('online.move.noSync')}</p>
        </CardContent>
      </Card>
    </section>
  );
}
