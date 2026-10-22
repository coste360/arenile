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
import { Skeleton } from '@arenile/shared/ui/skeleton';
import { cn } from '@arenile/shared/lib/utils';
import { CheckIcon, ChevronLeftIcon, ChevronRightIcon } from 'lucide-react';
import { Suspense, lazy, useId, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { IsoDateRange } from '@arenile/shared/ui/patterns/DatePicker';
import { STEPS, type Step } from './steps';

const MapStep = lazy(() => import('./MapStep'));
const WhenStep = lazy(() => import('./WhenStep'));

function StepLoading({ label }: { label: string }) {
  return (
    <div className="space-y-2">
      <span className="sr-only">{label}</span>
      <Skeleton aria-hidden="true" className="h-9 w-full sm:w-72" />
    </div>
  );
}

function ComingSoon({ children }: { children?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <Badge className="bg-highlight text-highlight-foreground">{t('common.inArrivo')}</Badge>
      {children ?? <p className="text-sm text-muted-foreground">{t('booking.comingSoonDetail')}</p>}
    </div>
  );
}

/**
 * The four steps of the public flow (spec 12.3). Steps whose backend is not
 * available yet render an honest "in arrivo" state instead of fake data.
 */
export function Stepper({ initial = 'when' }: { initial?: Step }) {
  const { t } = useTranslation();
  const [current, setCurrent] = useState<Step>(initial);
  const [dates, setDates] = useState<IsoDateRange | undefined>();
  const baseId = useId();
  const index = STEPS.indexOf(current);

  return (
    <section aria-label={t('booking.steps.label')} className="space-y-4">
      <ol className="grid grid-cols-4 gap-1.5 sm:gap-2">
        {STEPS.map((step, i) => {
          const active = step === current;
          const done = i < index;
          return (
            <li key={step}>
              <Button
                variant="ghost"
                onClick={() => setCurrent(step)}
                aria-current={active ? 'step' : undefined}
                aria-controls={`${baseId}-panel`}
                className={cn(
                  'h-auto w-full flex-col gap-1.5 rounded-lg px-1 py-2 text-xs font-medium whitespace-normal sm:text-sm',
                  active
                    ? 'bg-primary/10 text-primary-on-surface hover:bg-primary/15'
                    : 'text-muted-foreground',
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
                <span>{t(`booking.steps.${step}`)}</span>
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
          {current === 'when' && (
            <Suspense fallback={<StepLoading label={t('common.loading')} />}>
              <WhenStep value={dates} onChange={setDates} />
            </Suspense>
          )}
          <ComingSoon>
            {current === 'where' ? (
              <Suspense
                fallback={
                  <div className="space-y-2">
                    <span className="sr-only">{t('booking.mapLoading')}</span>
                    <Skeleton aria-hidden="true" className="h-48 w-full" />
                  </div>
                }
              >
                <MapStep />
              </Suspense>
            ) : undefined}
          </ComingSoon>
        </CardContent>
        <CardFooter className="justify-between gap-2">
          <Button
            variant="outline"
            disabled={index === 0}
            onClick={() => setCurrent(STEPS[index - 1] ?? 'when')}
          >
            <ChevronLeftIcon aria-hidden="true" />
            {t('common.back')}
          </Button>
          <Button
            disabled={index === STEPS.length - 1}
            onClick={() => setCurrent(STEPS[index + 1] ?? 'who')}
          >
            {t('common.next')}
            <ChevronRightIcon aria-hidden="true" />
          </Button>
        </CardFooter>
      </Card>
    </section>
  );
}
