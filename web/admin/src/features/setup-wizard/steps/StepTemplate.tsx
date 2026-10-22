import { Button, cn } from '@arenile/shared/ui';
import { useTranslation } from 'react-i18next';
import { localize } from '../../configuration/lib/i18nText';
import { useSetupTemplates, type SetupTemplate } from '../api';
import { TEMPLATE_CODES, type TemplateCode } from '../progress';
import type { StepProps } from './types';

/** Step 2: choice of one of the four templates of spec 15.2. */
export function StepTemplate({ ctx, progress, dispatch }: StepProps) {
  const { t } = useTranslation('setupWizard');
  const templates = useSetupTemplates(ctx.establishmentId);
  const byCode = new Map<string, SetupTemplate>(
    (templates.data ?? []).map((tpl) => [tpl.code, tpl]),
  );
  const selected = progress.template;

  const choose = (code: TemplateCode) => {
    dispatch({ type: 'setTemplate', template: code });
    dispatch({ type: 'complete' });
  };

  return (
    <div className="space-y-4">
      <p className="text-sm text-slate-600">{t('template.intro')}</p>
      {templates.isPending && <p className="text-sm text-slate-600">{t('loading')}</p>}
      <ul className="grid gap-4 sm:grid-cols-2" aria-label={t('template.listLabel')}>
        {TEMPLATE_CODES.map((code) => {
          const tpl = byCode.get(code);
          // Server texts when available; the bundled ones otherwise (also offline).
          const name = tpl ? localize(tpl.name, ctx.uiLocale, ctx.defaultLocale) : '';
          const description = tpl ? localize(tpl.description, ctx.uiLocale, ctx.defaultLocale) : '';
          const isSelected = selected === code;
          return (
            <li key={code}>
              <article
                aria-labelledby={`tpl-${code}`}
                className={cn(
                  'flex h-full flex-col gap-3 rounded-[var(--ar-radius)] border bg-white p-4 shadow-sm',
                  isSelected
                    ? 'border-[var(--ar-primary)] ring-2 ring-[var(--ar-primary)]'
                    : 'border-slate-200',
                )}
              >
                <h3 id={`tpl-${code}`} className="text-base font-semibold">
                  {name || t(`template.codes.${code}.name`)}
                </h3>
                <p className="text-sm text-slate-600">
                  {description || t(`template.codes.${code}.description`)}
                </p>
                {tpl && (
                  <dl className="grid grid-cols-2 gap-x-3 gap-y-1 text-xs text-slate-700">
                    {tpl.rows > 0 && (
                      <>
                        <dt>{t('template.layout')}</dt>
                        <dd>
                          {t('template.rowsBySpots', { rows: tpl.rows, spots: tpl.spots_per_row })}
                        </dd>
                      </>
                    )}
                    <dt>{t('template.zones')}</dt>
                    <dd>{tpl.zones.length}</dd>
                    <dt>{t('template.packs')}</dt>
                    <dd>
                      {tpl.packs
                        .map((p) => localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.code)
                        .join(', ') || '—'}
                    </dd>
                    <dt>{t('template.periods')}</dt>
                    <dd>{tpl.rate_periods.join(', ') || '—'}</dd>
                  </dl>
                )}
                <div className="mt-auto">
                  <Button
                    variant={isSelected ? 'primary' : 'outline'}
                    onClick={() => choose(code)}
                    aria-describedby={`tpl-${code}`}
                  >
                    {isSelected ? t('template.selected') : t('template.use')}
                  </Button>
                </div>
              </article>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
