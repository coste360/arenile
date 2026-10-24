import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { DownloadIcon, FileSpreadsheetIcon, FileTextIcon } from 'lucide-react';
import { Fragment } from 'react';
import { useTranslation } from 'react-i18next';
import { EXPORT_LISTS, exportListFileName, exportListUrl } from '../lib/exports';
import type { ResolvedPeriod } from '../lib/period';

/**
 * "Esporta elenchi" (ADR 0019 F): customers, bookings, season passes and
 * payments as CSV or Excel, same columns as the CSV reports (no extra
 * personal data), test bookings excluded by the server.
 */
export function ExportListsMenu({
  establishmentId,
  period,
}: {
  establishmentId: string;
  period: ResolvedPeriod;
}) {
  const { t } = useTranslation('reports');
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline">
          <DownloadIcon aria-hidden="true" />
          {t('exports.label')}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
          {t('exports.hint')}
        </DropdownMenuLabel>
        {EXPORT_LISTS.map((list) => (
          <Fragment key={list}>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t(`exports.${list}`)}</DropdownMenuLabel>
              {(['xlsx', 'csv'] as const).map((format) => (
                <DropdownMenuItem key={format} asChild>
                  <a
                    href={exportListUrl(establishmentId, list, format, period)}
                    download={exportListFileName(list, format, period)}
                  >
                    {format === 'xlsx' ? (
                      <FileSpreadsheetIcon aria-hidden="true" />
                    ) : (
                      <FileTextIcon aria-hidden="true" />
                    )}
                    {t(`exports.${format}`)}
                  </a>
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </Fragment>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
