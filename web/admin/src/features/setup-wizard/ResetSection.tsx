import { useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription, AlertTitle } from '@arenile/shared/ui/alert';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@arenile/shared/ui/alert-dialog';
import { Button } from '@arenile/shared/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@arenile/shared/ui/card';
import { Checkbox } from '@arenile/shared/ui/checkbox';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { Spinner } from '@arenile/shared/ui/spinner';
import { RotateCcwIcon, Trash2Icon, TriangleAlertIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useResetSetup, type SetupResetScope } from './api';

/** Same comparison as the server: trimmed, case-insensitive. */
function sameName(a: string, b: string): boolean {
  return a.trim().toLocaleLowerCase() === b.trim().toLocaleLowerCase();
}

/**
 * "Ricomincia da zero" of the setup wizard (owner only): start the
 * configuration over (refused while there are bookings or payments) or
 * reset the whole establishment, operational data included.
 */
export function ResetSection({
  establishmentId,
  establishmentName,
  hasOperations,
  onReset,
}: {
  establishmentId: string;
  establishmentName: string;
  hasOperations: boolean;
  onReset: () => void;
}) {
  const { t } = useTranslation('setupWizard');
  const [scope, setScope] = useState<SetupResetScope | null>(null);
  return (
    <Card className="gap-3 py-5">
      <CardHeader className="px-5">
        <CardTitle>
          <h2 className="text-sm font-semibold">{t('reset.title')}</h2>
        </CardTitle>
        <CardDescription>{t('reset.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2 px-5">
        <Button
          variant="outline"
          className="h-auto w-full justify-start py-2 text-left whitespace-normal"
          disabled={hasOperations}
          aria-describedby={hasOperations ? 'reset-configuration-hint' : undefined}
          onClick={() => setScope('configuration')}
        >
          <RotateCcwIcon aria-hidden="true" />
          {t('reset.configuration.action')}
        </Button>
        {hasOperations && (
          <p id="reset-configuration-hint" className="text-xs text-muted-foreground">
            {t('reset.configuration.blocked')}
          </p>
        )}
        <Button
          variant="outline"
          className="h-auto w-full justify-start py-2 text-left whitespace-normal text-destructive"
          onClick={() => setScope('everything')}
        >
          <Trash2Icon aria-hidden="true" />
          {t('reset.everything.action')}
        </Button>
      </CardContent>
      <AlertDialog open={scope !== null} onOpenChange={(o) => !o && setScope(null)}>
        <AlertDialogContent>
          {scope && (
            <ResetForm
              key={scope}
              scope={scope}
              establishmentId={establishmentId}
              establishmentName={establishmentName}
              onDone={() => {
                setScope(null);
                onReset();
              }}
            />
          )}
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}

function ResetForm({
  scope,
  establishmentId,
  establishmentName,
  onDone,
}: {
  scope: SetupResetScope;
  establishmentId: string;
  establishmentName: string;
  onDone: () => void;
}) {
  const { t } = useTranslation('setupWizard');
  const problemText = useProblemText();
  const reset = useResetSetup(establishmentId);
  const nameId = useId();
  const ackId = useId();
  const [name, setName] = useState('');
  const [ack, setAck] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const ready = sameName(name, establishmentName) && (scope === 'configuration' || ack);
  const submit = async () => {
    setFailure(null);
    try {
      await reset.mutateAsync({ scope, confirmName: name });
      toast.success(t(`reset.${scope}.done`));
      onDone();
    } catch (err) {
      setFailure(problemText(err));
    }
  };
  return (
    <form
      noValidate
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (ready) void submit();
      }}
    >
      <AlertDialogHeader>
        <AlertDialogTitle>{t(`reset.${scope}.title`)}</AlertDialogTitle>
        <AlertDialogDescription>{t(`reset.${scope}.deletes`)}</AlertDialogDescription>
      </AlertDialogHeader>
      <Alert variant="destructive">
        <TriangleAlertIcon aria-hidden="true" />
        <AlertTitle>{t('reset.irreversible')}</AlertTitle>
        <AlertDescription>{t('reset.keeps')}</AlertDescription>
      </Alert>
      {failure && (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      )}
      <div className="grid gap-2">
        <Label htmlFor={nameId}>{t('reset.typeName', { name: establishmentName })}</Label>
        <Input
          id={nameId}
          autoComplete="off"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      {scope === 'everything' && (
        <div className="flex items-start gap-2">
          <Checkbox id={ackId} checked={ack} onCheckedChange={(v) => setAck(v === true)} />
          <Label htmlFor={ackId} className="leading-snug font-normal">
            {t('reset.everything.ack')}
          </Label>
        </div>
      )}
      <AlertDialogFooter>
        <AlertDialogCancel disabled={reset.isPending}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </AlertDialogCancel>
        <Button type="submit" variant="destructive" disabled={!ready || reset.isPending}>
          {reset.isPending && <Spinner />}
          {t(`reset.${scope}.confirm`)}
        </Button>
      </AlertDialogFooter>
    </form>
  );
}
