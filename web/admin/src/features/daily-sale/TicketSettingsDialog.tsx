import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Button } from '@arenile/shared/ui/button';
import { Label } from '@arenile/shared/ui/label';
import { FormDialog, SegmentedControl } from '@arenile/shared/ui/patterns';
import { Spinner } from '@arenile/shared/ui/spinner';
import { Switch } from '@arenile/shared/ui/switch';
import { CircleAlertIcon } from 'lucide-react';
import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useUpdateDailySaleSettings, type DailySaleSettings, type TicketPrintFormat } from './api';
import { PRINT_FORMATS } from './lib';
import { useSaleProblemText } from './useSaleProblemText';

export interface TicketSettingsDialogProps {
  establishmentId: string;
  settings: DailySaleSettings;
  onClose: () => void;
}

/**
 * Ticket settings (establishment.configure): default print format, print
 * right after the sale, zone-first sales. The footer text is kept as is.
 */
export function TicketSettingsDialog({
  establishmentId,
  settings,
  onClose,
}: TicketSettingsDialogProps) {
  const { t } = useTranslation('dailySale');
  const id = useId();
  const update = useUpdateDailySaleSettings(establishmentId);
  const problemText = useSaleProblemText();
  const [format, setFormat] = useState<TicketPrintFormat>(settings.print_format);
  const [autoPrint, setAutoPrint] = useState(settings.auto_print);
  const [zoneFirst, setZoneFirst] = useState(settings.zone_first_enabled);

  const save = () =>
    update.mutate(
      {
        version: settings.version,
        body: {
          print_format: format,
          auto_print: autoPrint,
          zone_first_enabled: zoneFirst,
          footer_text: settings.footer_text,
        },
      },
      {
        onSuccess: () => {
          toast.success(t('settings.saved'));
          onClose();
        },
      },
    );

  return (
    <FormDialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t('settings.title')}
      description={t('settings.description')}
    >
      <div className="space-y-5">
        <div className="space-y-2">
          <p className="text-sm font-medium">{t('settings.format')}</p>
          <SegmentedControl
            label={t('settings.format')}
            value={format}
            onValueChange={(v) => setFormat(v as TicketPrintFormat)}
            options={PRINT_FORMATS.map((f) => ({ value: f, label: t(`format.${f}`) }))}
            collapseOnNarrow
          />
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor={`${id}-auto`}>{t('settings.autoPrint')}</Label>
            <p className="text-sm text-muted-foreground">{t('settings.autoPrintHint')}</p>
          </div>
          <Switch id={`${id}-auto`} checked={autoPrint} onCheckedChange={setAutoPrint} />
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor={`${id}-zone`}>{t('settings.zoneFirst')}</Label>
            <p className="text-sm text-muted-foreground">{t('settings.zoneFirstHint')}</p>
          </div>
          <Switch id={`${id}-zone`} checked={zoneFirst} onCheckedChange={setZoneFirst} />
        </div>
        {update.error && (
          <Alert variant="destructive">
            <CircleAlertIcon aria-hidden="true" />
            <AlertDescription>{problemText(update.error)}</AlertDescription>
          </Alert>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={onClose}>
            {t('common.actions.cancel', { ns: 'translation' })}
          </Button>
          <Button onClick={save} disabled={update.isPending}>
            {update.isPending && <Spinner />}
            {t('common.actions.save', { ns: 'translation' })}
          </Button>
        </div>
      </div>
    </FormDialog>
  );
}
