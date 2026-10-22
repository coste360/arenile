import { Suspense, lazy, useId, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { STEPS, type Step } from './steps';

const MapStep = lazy(() => import('./MapStep'));

function ComingSoon({ children }: { children?: ReactNode }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3">
      <p className="inline-flex rounded-full bg-[var(--ar-accent)] px-3 py-1 text-xs font-semibold text-[var(--ar-accent-foreground)]">
        {t('common.inArrivo')}
      </p>
      {children ?? <p className="text-sm text-slate-600">{t('booking.comingSoonDetail')}</p>}
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
  const baseId = useId();
  const index = STEPS.indexOf(current);

  return (
    <section aria-label={t('booking.steps.label')} className="space-y-4">
      <ol className="grid grid-cols-4 gap-1 sm:gap-2">
        {STEPS.map((step, i) => {
          const active = step === current;
          return (
            <li key={step}>
              <button
                type="button"
                onClick={() => setCurrent(step)}
                aria-current={active ? 'step' : undefined}
                aria-controls={`${baseId}-panel`}
                className={
                  'flex w-full flex-col items-center gap-1 rounded-[var(--ar-radius)] px-1 py-2 text-center text-xs font-medium sm:text-sm ' +
                  (active
                    ? 'bg-[var(--ar-primary)] text-[var(--ar-primary-foreground)]'
                    : 'bg-slate-100 text-slate-700 hover:bg-slate-200')
                }
              >
                <span aria-hidden="true" className="text-base font-semibold">
                  {i + 1}
                </span>
                <span>{t(`booking.steps.${step}`)}</span>
              </button>
            </li>
          );
        })}
      </ol>
      <div
        id={`${baseId}-panel`}
        role="region"
        aria-labelledby={`${baseId}-title`}
        className="space-y-3 rounded-[var(--ar-radius)] border border-slate-200 p-4"
      >
        <h2 id={`${baseId}-title`} className="text-lg font-semibold">
          {index + 1}. {t(`booking.steps.${current}`)}
        </h2>
        <p className="text-sm text-slate-700">{t(`booking.stepText.${current}`)}</p>
        <ComingSoon>
          {current === 'where' ? (
            <Suspense
              fallback={<p className="text-sm text-slate-600">{t('booking.mapLoading')}</p>}
            >
              <MapStep />
            </Suspense>
          ) : undefined}
        </ComingSoon>
        <div className="flex justify-between gap-2 pt-2">
          <button
            type="button"
            disabled={index === 0}
            onClick={() => setCurrent(STEPS[index - 1] ?? 'when')}
            className="h-10 rounded-[var(--ar-radius)] border border-slate-300 px-4 text-sm font-medium disabled:opacity-40"
          >
            {t('common.back')}
          </button>
          <button
            type="button"
            disabled={index === STEPS.length - 1}
            onClick={() => setCurrent(STEPS[index + 1] ?? 'who')}
            className="h-10 rounded-[var(--ar-radius)] bg-[var(--ar-primary)] px-4 text-sm font-medium text-[var(--ar-primary-foreground)] disabled:opacity-40"
          >
            {t('common.next')}
          </button>
        </div>
      </div>
    </section>
  );
}
