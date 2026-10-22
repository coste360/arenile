import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  FieldContent,
  FieldDescription,
  FieldLabel,
  FieldTitle,
  Field as UiField,
} from '@arenile/shared/ui/field';
import { RadioGroup, RadioGroupItem } from '@arenile/shared/ui/radio-group';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import {
  ArrowRightIcon,
  BoxesIcon,
  LayoutTemplateIcon,
  Rows3Icon,
  WavesLadderIcon,
  type LucideIcon,
} from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { localize } from '../../configuration/lib/i18nText';
import { useSetupTemplates, type SetupTemplate } from '../api';
import { TEMPLATE_CODES, type TemplateCode } from '../progress';
import type { StepProps } from './types';

const ILLUSTRATIONS: Record<TemplateCode, LucideIcon> = {
  classic_rows: Rows3Icon,
  quantity_beach: BoxesIcon,
  lido_with_pool: WavesLadderIcon,
  empty: LayoutTemplateIcon,
};

/** Step 2: choice of one of the four templates of spec 15.2 (cards in a RadioGroup). */
export function StepTemplate({ ctx, progress, dispatch }: StepProps) {
  const { t } = useTranslation('setupWizard');
  const templates = useSetupTemplates(ctx.establishmentId);
  const byCode = new Map<string, SetupTemplate>(
    (templates.data ?? []).map((tpl) => [tpl.code, tpl]),
  );
  const [choice, setChoice] = useState<TemplateCode>(progress.template ?? 'classic_rows');

  const confirm = () => {
    dispatch({ type: 'setTemplate', template: choice });
    dispatch({ type: 'complete' });
  };

  return (
    <div className="space-y-5">
      <p className="text-sm text-muted-foreground">{t('template.intro')}</p>
      <RadioGroup
        value={choice}
        onValueChange={(v) => setChoice(v as TemplateCode)}
        aria-label={t('template.listLabel')}
        className="grid gap-4 sm:grid-cols-2"
      >
        {TEMPLATE_CODES.map((code) => {
          const tpl = byCode.get(code);
          // Server texts when available; the bundled ones otherwise (also offline).
          const name = tpl ? localize(tpl.name, ctx.uiLocale, ctx.defaultLocale) : '';
          const description = tpl ? localize(tpl.description, ctx.uiLocale, ctx.defaultLocale) : '';
          const Illustration = ILLUSTRATIONS[code];
          return (
            <FieldLabel key={code} htmlFor={`tpl-${code}`} className="h-full">
              <UiField orientation="horizontal" className="h-full items-start">
                <span className="flex size-11 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-map-sea/40 to-map-sand text-primary-on-surface">
                  <Illustration aria-hidden="true" className="size-5" />
                </span>
                <FieldContent>
                  <FieldTitle className="flex flex-wrap items-center gap-2">
                    {name || t(`template.codes.${code}.name`)}
                    {progress.template === code && (
                      <Badge variant="secondary">{t('template.current')}</Badge>
                    )}
                  </FieldTitle>
                  <FieldDescription>
                    {description || t(`template.codes.${code}.description`)}
                  </FieldDescription>
                  {templates.isPending && <Skeleton className="mt-1 h-10 w-full" />}
                  {tpl && (
                    <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                      {tpl.rows > 0 && (
                        <>
                          <dt>{t('template.layout')}</dt>
                          <dd className="text-foreground">
                            {t('template.rowsBySpots', {
                              rows: tpl.rows,
                              spots: tpl.spots_per_row,
                            })}
                          </dd>
                        </>
                      )}
                      <dt>{t('template.zones')}</dt>
                      <dd className="text-foreground">{tpl.zones.length}</dd>
                      <dt>{t('template.packs')}</dt>
                      <dd className="text-foreground">
                        {tpl.packs
                          .map((p) => localize(p.name, ctx.uiLocale, ctx.defaultLocale) || p.code)
                          .join(', ') || '—'}
                      </dd>
                      <dt>{t('template.periods')}</dt>
                      <dd className="text-foreground">{tpl.rate_periods.join(', ') || '—'}</dd>
                    </dl>
                  )}
                </FieldContent>
                <RadioGroupItem value={code} id={`tpl-${code}`} />
              </UiField>
            </FieldLabel>
          );
        })}
      </RadioGroup>
      <Button onClick={confirm}>
        {t('template.use')}
        <ArrowRightIcon aria-hidden="true" />
      </Button>
    </div>
  );
}
