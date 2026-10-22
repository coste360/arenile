import { Button } from '@arenile/shared/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@arenile/shared/ui/card';
import { Progress } from '@arenile/shared/ui/progress';
import { cn } from '@arenile/shared/lib/utils';
import { EmptyState, QueryState } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import {
  ArrowLeftIcon,
  CheckIcon,
  CircleCheckIcon,
  CircleMinusIcon,
  EuroIcon,
  GlobeIcon,
  LayoutTemplateIcon,
  LockIcon,
  MapIcon,
  PartyPopperIcon,
  ReceiptTextIcon,
  RotateCcwIcon,
  Rows3Icon,
  Building2Icon,
  SkipForwardIcon,
  WalletIcon,
  type LucideIcon,
} from 'lucide-react';
import { useEffect, useReducer, useState, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigContext, type ConfigContext } from '../configuration/api/useConfigContext';
import { ConfigPage } from '../configuration/components/ConfigPage';
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

const STEP_ICONS: Record<StepId, LucideIcon> = {
  details: Building2Icon,
  template: LayoutTemplateIcon,
  rows: Rows3Icon,
  pricelist: EuroIcon,
  publicPage: GlobeIcon,
  payments: WalletIcon,
};

/** Configurazione guidata (spec 15.2): six skippable steps, progress kept per establishment. */
export function SetupWizardPage() {
  const { t } = useTranslation('setupWizard');
  const ctx = useConfigContext();
  const status = useSetupStatus(ctx.establishmentId);

  if (ctx.isLoading || status.isPending) {
    return (
      <ConfigPage title={t('title')}>
        <QueryState isLoading error={null} rows={5} />
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
        <EmptyState icon={LockIcon} title={t('forbidden')} />
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
  const StepIcon = STEP_ICONS[stepId];

  return (
    <ConfigPage title={t('title')} description={t('description')}>
      <div className="grid items-start gap-6 lg:grid-cols-[17rem_1fr]">
        <Card className="gap-4 py-5 lg:sticky lg:top-4">
          <CardContent className="space-y-4 px-5">
            <div className="space-y-2">
              <div className="flex items-baseline justify-between text-sm">
                <span className="font-medium">{t('progress')}</span>
                <span className="text-muted-foreground tabular-nums">
                  {handled}/{STEPS.length}
                </span>
              </div>
              <Progress
                value={(handled / STEPS.length) * 100}
                max={100}
                aria-label={t('progress')}
                getValueLabel={() => t('progressText', { done: handled, total: STEPS.length })}
              />
            </div>
            <nav aria-label={t('stepsLabel')}>
              <ol className="space-y-1">
                {STEPS.map((s, i) => {
                  const state = progress.states[s];
                  const current = i === progress.current && !progress.finished;
                  const Icon = STEP_ICONS[s];
                  return (
                    <li key={s}>
                      <Button
                        variant={current ? 'secondary' : 'ghost'}
                        onClick={() => dispatch({ type: 'goto', step: i })}
                        aria-current={current ? 'step' : undefined}
                        className="h-auto w-full justify-start gap-3 px-2 py-2 text-left"
                      >
                        <span
                          aria-hidden="true"
                          className={cn(
                            'flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-semibold',
                            state === 'done' && 'border-primary bg-primary text-primary-foreground',
                            state === 'skipped' && 'border-dashed text-muted-foreground',
                            state === 'pending' &&
                              current &&
                              'border-primary text-primary-on-surface',
                          )}
                        >
                          {state === 'done' ? (
                            <CheckIcon className="size-3.5" />
                          ) : state === 'skipped' ? (
                            <CircleMinusIcon className="size-3.5" />
                          ) : (
                            i + 1
                          )}
                        </span>
                        <span className="flex min-w-0 flex-1 items-center gap-2">
                          <Icon aria-hidden="true" className="text-muted-foreground" />
                          <span className="truncate">{t(`steps.${s}`)}</span>
                        </span>
                        <span className="sr-only">({t(`state.${state}`)})</span>
                      </Button>
                    </li>
                  );
                })}
              </ol>
            </nav>
          </CardContent>
        </Card>

        {progress.finished ? (
          <Finished ctx={ctx} progress={progress} onRestart={() => dispatch({ type: 'restart' })} />
        ) : (
          <Card>
            <CardHeader className="flex flex-row items-center gap-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary-on-surface">
                <StepIcon aria-hidden="true" className="size-5" />
              </span>
              <div>
                <CardDescription>
                  {t('stepCounter', { n: progress.current + 1, total: STEPS.length })}
                </CardDescription>
                <CardTitle>
                  <h2 id="step-title" className="text-lg font-semibold">
                    <span className="sr-only">
                      {t('stepHeading', {
                        n: progress.current + 1,
                        total: STEPS.length,
                        name: t(`steps.${stepId}`),
                      })}
                    </span>
                    <span aria-hidden="true">{t(`steps.${stepId}`)}</span>
                  </h2>
                </CardTitle>
              </div>
            </CardHeader>
            <CardContent>
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
            </CardContent>
            <CardFooter className="flex-wrap justify-between gap-2 border-t">
              <Button
                variant="ghost"
                onClick={() => dispatch({ type: 'back' })}
                disabled={progress.current === 0}
              >
                <ArrowLeftIcon aria-hidden="true" />
                {t('back')}
              </Button>
              {progress.current === STEPS.length - 1 ? (
                <Button onClick={() => dispatch({ type: 'complete' })}>
                  <CheckIcon aria-hidden="true" />
                  {t('finish')}
                </Button>
              ) : (
                <Button variant="outline" onClick={() => dispatch({ type: 'skip' })}>
                  <SkipForwardIcon aria-hidden="true" />
                  {t('skip')}
                </Button>
              )}
            </CardFooter>
          </Card>
        )}
      </div>
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
  const params = { establishmentId: ctx.establishmentId };
  const links = [
    { key: 'maps', icon: MapIcon, to: '/e/$establishmentId/maps' as const },
    { key: 'pricing', icon: ReceiptTextIcon, to: '/e/$establishmentId/pricing' as const },
    {
      key: 'settings',
      icon: Building2Icon,
      to: '/e/$establishmentId/config/establishment' as const,
    },
  ];
  return (
    <Card>
      <CardHeader className="items-center text-center">
        <span className="mx-auto flex size-12 items-center justify-center rounded-full bg-highlight text-highlight-foreground">
          <PartyPopperIcon aria-hidden="true" className="size-6" />
        </span>
        <CardTitle>
          <h2 className="text-xl font-semibold">{t('finished.title')}</h2>
        </CardTitle>
        <CardDescription>{t('finished.text')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <section aria-labelledby="summary-title" className="space-y-2">
          <h3 id="summary-title" className="text-sm font-medium">
            {t('finished.summary')}
          </h3>
          <ul className="grid gap-2 sm:grid-cols-2">
            {STEPS.map((s) => {
              const done = progress.states[s] === 'done';
              const Icon = STEP_ICONS[s];
              return (
                <li key={s} className="flex items-center gap-3 rounded-lg border px-3 py-2 text-sm">
                  <Icon aria-hidden="true" className="size-4 text-muted-foreground" />
                  <span className="flex-1">{t(`steps.${s}`)}</span>
                  {done ? (
                    <CircleCheckIcon aria-hidden="true" className="size-4 text-success" />
                  ) : (
                    <CircleMinusIcon aria-hidden="true" className="size-4 text-warning" />
                  )}
                  <span className="text-xs text-muted-foreground">
                    {t(`state.${progress.states[s]}`)}
                  </span>
                </li>
              );
            })}
          </ul>
          {STEPS.some((s) => progress.states[s] === 'skipped') && (
            <p className="text-sm text-muted-foreground">
              {t('finished.skipped', {
                steps: STEPS.filter((s) => progress.states[s] === 'skipped')
                  .map((s) => t(`steps.${s}`))
                  .join(', '),
              })}
            </p>
          )}
        </section>
        <ul className="grid gap-2 sm:grid-cols-3">
          {links.map(({ key, icon: Icon, to }) => (
            <li key={key}>
              <Button asChild variant="outline" className="w-full">
                <Link to={to} params={params} search={key === 'pricing' ? {} : undefined}>
                  <Icon aria-hidden="true" />
                  {t(`finished.${key}`)}
                </Link>
              </Button>
            </li>
          ))}
        </ul>
      </CardContent>
      <CardFooter className="justify-center border-t">
        <Button variant="ghost" onClick={onRestart}>
          <RotateCcwIcon aria-hidden="true" />
          {t('finished.restart')}
        </Button>
      </CardFooter>
    </Card>
  );
}
