import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { cn } from '@arenile/shared/lib/utils';
import { CheckIcon, ChevronLeftIcon, ChevronRightIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { IsoDateRange } from '@arenile/shared/ui/patterns/DatePicker';
import { STEPS, type Step } from './steps';
import { WhenStep, type WhenStepProps } from './WhenStep';

/** Steps whose backend exists; the others are shown disabled with "In arrivo". */
const AVAILABLE: readonly Step[] = ['when'];

/**
 * The four steps of the public flow (spec 12.3). Only "Quando" works today:
 * the later steps are visible but disabled, and the footer says so instead
 * of leading into empty pages.
 */
export function Stepper({
  initial = 'when',
  season,
}: {
  initial?: Step;
  season?: WhenStepProps['season'];
}) {
  const { t } = useTranslation();
  const [current, setCurrent] = useState<Step>(initial);
  const [dates, setDates] = useState<IsoDateRange | undefined>();
  const baseId = useId();
  const index = STEPS.indexOf(current);
  const next = STEPS[index + 1];
  const canGoNext = !!next && AVAILABLE.includes(next);

  return (
    <section aria-label={t('booking.steps.label')} className="space-y-4">
      <ol className="grid grid-cols-4 gap-1.5 sm:gap-2">
        {STEPS.map((step, i) => {
          const active = step === current;
          const done = i < index;
          const available = AVAILABLE.includes(step);
          return (
            <li key={step}>
              <Button
                variant="ghost"
                onClick={() => available && setCurrent(step)}
                aria-current={active ? 'step' : undefined}
                aria-controls={`${baseId}-panel`}
                aria-disabled={available ? undefined : true}
                tabIndex={available ? undefined : -1}
                className={cn(
                  'h-auto w-full flex-col gap-1.5 rounded-lg px-1 py-2 text-xs font-medium whitespace-normal sm:text-sm',
                  active
                    ? 'bg-primary/10 text-primary-on-surface hover:bg-primary/15'
                    : 'text-muted-foreground',
                  !available && 'cursor-default opacity-60 hover:bg-transparent',
                )}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'flex size-7 items-center justify-center rounded-full border text-sm font-semibold',
                    active && 'border-primary bg-primary text-primary-foreground',
                    done && 'border-primary text-primary-on-surface',
                  )}
                >
                  {done ? <CheckIcon className="size-4" /> : i + 1}
                </span>
                <span>
                  {t(`booking.steps.${step}`)}
                  {!available && <span className="sr-only"> ({t('common.inArrivo')})</span>}
                </span>
              </Button>
            </li>
          );
        })}
      </ol>
      <Card id={`${baseId}-panel`} role="region" aria-labelledby={`${baseId}-title`}>
        <CardHeader>
          <CardTitle>
            <h2 id={`${baseId}-title`} className="text-lg">
              {index + 1}. {t(`booking.steps.${current}`)}
            </h2>
          </CardTitle>
          <CardDescription>{t(`booking.stepText.${current}`)}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {current === 'when' && <WhenStep value={dates} onChange={setDates} season={season} />}
        </CardContent>
        <CardFooter className="flex-wrap items-center justify-between gap-3 border-t">
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <Badge className="bg-highlight text-highlight-foreground">{t('common.inArrivo')}</Badge>
            {t('booking.nextStepsSoon')}
          </p>
          <div className="flex gap-2">
            <Button
              variant="outline"
              disabled={index === 0}
              onClick={() => setCurrent(STEPS[index - 1] ?? 'when')}
            >
              <ChevronLeftIcon aria-hidden="true" />
              {t('common.back')}
            </Button>
            <Button disabled={!canGoNext} onClick={() => next && setCurrent(next)}>
              {t('common.next')}
              <ChevronRightIcon aria-hidden="true" />
            </Button>
          </div>
        </CardFooter>
      </Card>
    </section>
  );
}
