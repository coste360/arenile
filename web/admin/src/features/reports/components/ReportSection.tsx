import type { ApiProblem } from '@arenile/shared/api';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import { NativeSelect, NativeSelectOption } from '@arenile/shared/ui/native-select';
import { QueryState, SectionCard } from '@arenile/shared/ui/patterns';
import { DownloadIcon, FileSpreadsheetIcon } from 'lucide-react';
import { useId, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toXlsx } from '../lib/exports';

export interface ReportSectionProps {
  title: string;
  description?: ReactNode;
  /** CSV export link of exactly what the section shows. */
  csv?: { href: string; fileName: string };
  /** Grouping / sorting controls, in one row above the content. */
  controls?: ReactNode;
  query: { isPending: boolean; error: ApiProblem | null; refetch: () => unknown };
  children: ReactNode;
}

/**
 * Card of one report: title, CSV export, controls, then the content once the
 * query has data (QueryState for loading and errors). Tables scroll inside
 * the card on narrow screens, never the page.
 */
export function ReportSection({
  title,
  description,
  csv,
  controls,
  query,
  children,
}: ReportSectionProps) {
  const { t } = useTranslation('reports');
  return (
    <SectionCard
      title={title}
      description={description}
      actions={
        csv && (
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" asChild>
              <a
                href={csv.href}
                download={csv.fileName}
                aria-label={t('csv.aria', { report: title })}
              >
                <DownloadIcon aria-hidden="true" />
                {t('csv.label')}
              </a>
            </Button>
            <Button variant="outline" asChild>
              <a
                href={toXlsx(csv.href)}
                download={csv.fileName.replace(/\.csv$/, '.xlsx')}
                aria-label={t('xlsx.aria', { report: title })}
              >
                <FileSpreadsheetIcon aria-hidden="true" />
                {t('xlsx.label')}
              </a>
            </Button>
          </div>
        )
      }
    >
      <div className="min-w-0 space-y-4">
        {controls && <div className="flex flex-wrap items-end gap-4">{controls}</div>}
        {query.error ? (
          <QueryState isLoading={false} error={query.error} onRetry={() => void query.refetch()} />
        ) : query.isPending ? (
          <QueryState isLoading error={null} rows={4} />
        ) : (
          children
        )}
      </div>
    </SectionCard>
  );
}

export interface ChoiceSelectProps<V extends string> {
  label: string;
  value: V;
  options: readonly { value: V; label: string; disabled?: boolean }[];
  onChange: (value: V) => void;
}

/** Labelled native select for a report option (group by, basis, sort). */
export function ChoiceSelect<V extends string>({
  label,
  value,
  options,
  onChange,
}: ChoiceSelectProps<V>) {
  const id = useId();
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <NativeSelect id={id} value={value} onChange={(e) => onChange(e.target.value as V)}>
        {options.map((o) => (
          <NativeSelectOption key={o.value} value={o.value} disabled={o.disabled}>
            {o.label}
          </NativeSelectOption>
        ))}
      </NativeSelect>
    </div>
  );
}
