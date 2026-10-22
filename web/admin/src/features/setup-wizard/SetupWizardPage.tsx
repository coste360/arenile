import { Button, Card, CardContent, cn } from '@arenile/shared/ui';
import { Link } from '@tanstack/react-router';
import { useEffect, useReducer, useState, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext, type ConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage, QueryState } from '../configuration/components/ConfigPage';
import { useSetupStatus, type SetupStatus } from './api';
import {
  STEPS,
  browserStorage,
  initialProgress,
  loadProgress,
  saveProgress,
  stepsHandled,
  wizardReducer,
  type StepId,
  type WizardProgress,
} from './progress';
import { StepDetails } from './steps/StepDetails';
import { StepPayments } from './steps/StepPayments';
import { StepPricelist } from './steps/StepPricelist';
import { StepPublicPage } from './steps/StepPublicPage';
import { StepRows } from './steps/StepRows';
import { StepTemplate } from './steps/StepTemplate';
import type { StepProps } from './steps/types';

const STEP_COMPONENTS: Record<StepId, ComponentType<StepProps>> = {
  details: StepDetails,
  template: StepTemplate,
  rows: StepRows,
  pricelist: StepPricelist,
  publicPage: StepPublicPage,
  payments: StepPayments,
};

/** Configurazione guidata (spec 15.2): six skippable steps, progress kept per establishment. */
export function SetupWizardPage() {
  const { t } = useTranslation('setupWizard');
  const ctx = useConfigContext();
  const status = useSetupStatus(ctx.establishmentId);

  if (ctx.isLoading || status.isPending) {
    return (
      <ConfigPage title={t('title')}>
        <QueryState isLoading error={null} />
      </ConfigPage>
    );
  }
  if (!ctx.establishment) {
    return (
      <ConfigPage title={t('title')}>
        <QueryState isLoading={false} error={ctx.error} onRetry={ctx.refetch} />
      </ConfigPage>
    );
  }
  if (!ctx.canConfigure) {
    return (
      <ConfigPage title={t('title')}>
        <p
          role="alert"
          className="rounded-md border border-slate-200 bg-slate-100 px-3 py-2 text-sm"
        >
          {t('forbidden')}
        </p>
      </ConfigPage>
    );
  }
  // The status is only a hint for resuming: if it fails the wizard still works.
  return <Wizard key={ctx.establishmentId} ctx={ctx} status={status.data} />;
}

function Wizard({ ctx, status }: { ctx: ConfigContext; status: SetupStatus | undefined }) {
  const { t } = useTranslation('setupWizard');
  const establishmentId = ctx.establishmentId;
  const [progress, dispatch] = useReducer(
    wizardReducer,
    undefined,
    (): WizardProgress =>
      loadProgress(browserStorage(), establishmentId) ?? initialProgress(status),
  );
  const [version, setVersion] = useState(ctx.establishment?.version ?? 0);

  // Persist every change so leaving and re-entering resumes where the user was.
  useEffect(() => {
    saveProgress(browserStorage(), establishmentId, progress);
  }, [establishmentId, progress]);

  const stepId = STEPS[progress.current] as StepId;
  const Step = STEP_COMPONENTS[stepId];
  const handled = stepsHandled(progress);

  return (
    <ConfigPage title={t('title')} description={t('description')}>
      <div className="space-y-2">
        <div
          role="progressbar"
          aria-label={t('progress')}
          aria-valuemin={0}
          aria-valuemax={STEPS.length}
          aria-valuenow={handled}
          aria-valuetext={t('progressText', { done: handled, total: STEPS.length })}
          className="h-2 overflow-hidden rounded-full bg-slate-200"
        >
          <div
            className="h-full bg-[var(--ar-primary)] transition-all"
            style={{ width: `${(handled / STEPS.length) * 100}%` }}
          />
        </div>
        <nav aria-label={t('stepsLabel')}>
          <ol className="flex flex-wrap gap-2">
            {STEPS.map((s, i) => {
              const state = progress.states[s];
              const current = i === progress.current && !progress.finished;
              return (
                <li key={s}>
                  <button
                    type="button"
                    onClick={() => dispatch({ type: 'goto', step: i })}
                    aria-current={current ? 'step' : undefined}
                    className={cn(
                      'flex items-center gap-2 rounded-full border px-3 py-1 text-sm',
                      current
                        ? 'border-[var(--ar-primary)] bg-[var(--ar-primary)] text-[var(--ar-primary-foreground)]'
                        : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50',
                    )}
                  >
                    <span aria-hidden="true" className="font-semibold">
                      {state === 'done' ? '✓' : state === 'skipped' ? '–' : i + 1}
                    </span>
                    <span>{t(`steps.${s}`)}</span>
                    <span className="sr-only">({t(`state.${state}`)})</span>
                  </button>
                </li>
              );
            })}
          </ol>
        </nav>
      </div>

      {progress.finished ? (
        <Finished ctx={ctx} progress={progress} onRestart={() => dispatch({ type: 'restart' })} />
      ) : (
        <Card>
          <CardContent className="space-y-4 pt-5">
            <h2 className="text-lg font-semibold" id="step-title">
              {t('stepHeading', {
                n: progress.current + 1,
                total: STEPS.length,
                name: t(`steps.${stepId}`),
              })}
            </h2>
            <section aria-labelledby="step-title">
              <Step
                key={stepId}
                ctx={ctx}
                progress={progress}
                dispatch={dispatch}
                status={status}
                version={version}
                onVersion={setVersion}
              />
            </section>
            <div className="flex flex-wrap justify-between gap-2 border-t border-slate-200 pt-4">
              <Button
                variant="ghost"
                onClick={() => dispatch({ type: 'back' })}
                disabled={progress.current === 0}
              >
                {t('back')}
              </Button>
              <Button variant="outline" onClick={() => dispatch({ type: 'skip' })}>
                {progress.current === STEPS.length - 1 ? t('skipAndFinish') : t('skip')}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </ConfigPage>
  );
}

function Finished({
  ctx,
  progress,
  onRestart,
}: {
  ctx: ConfigContext;
  progress: WizardProgress;
  onRestart: () => void;
}) {
  const { t } = useTranslation('setupWizard');
  const skipped = STEPS.filter((s) => progress.states[s] === 'skipped');
  const params = { establishmentId: ctx.establishmentId };
  return (
    <Card>
      <CardContent className="space-y-4 pt-5">
        <h2 className="text-lg font-semibold">{t('finished.title')}</h2>
        <p className="text-sm text-slate-700">{t('finished.text')}</p>
        {skipped.length > 0 && (
          <p className="text-sm text-amber-800">
            {t('finished.skipped', { steps: skipped.map((s) => t(`steps.${s}`)).join(', ') })}
          </p>
        )}
        <ul className="flex flex-wrap gap-2">
          <li>
            <Button asChild variant="outline" size="sm">
              <Link to="/e/$establishmentId/maps" params={params}>
                {t('finished.maps')}
              </Link>
            </Button>
          </li>
          <li>
            <Button asChild variant="outline" size="sm">
              <Link to="/e/$establishmentId/pricing" params={params} search={{}}>
                {t('finished.pricing')}
              </Link>
            </Button>
          </li>
          <li>
            <Button asChild variant="outline" size="sm">
              <Link to="/e/$establishmentId/config/establishment" params={params}>
                {t('finished.settings')}
              </Link>
            </Button>
          </li>
        </ul>
        <Button variant="ghost" onClick={onRestart}>
          {t('finished.restart')}
        </Button>
      </CardContent>
    </Card>
  );
}
