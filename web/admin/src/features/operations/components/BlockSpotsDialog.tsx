import { mapProblemToForm, useProblemText } from '@arenile/shared/api';
import { Alert, AlertDescription } from '@arenile/shared/ui/alert';
import { Badge } from '@arenile/shared/ui/badge';
import { Field, FieldError, FieldLabel } from '@arenile/shared/ui/field';
import { Input } from '@arenile/shared/ui/input';
import {
  DateRangePicker,
  FormActions,
  FormDialog,
  SegmentedControl,
} from '@arenile/shared/ui/patterns';
import { zodResolver } from '@hookform/resolvers/zod';
import { CircleAlertIcon } from 'lucide-react';
import { useId, useRef, useState } from 'react';
import { useController, useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { newIdempotencyKey } from '../../configuration/lib/validation';
import { useCreateSpotBlocks } from '../api/operations';
import { blockSchema, blockValues, toSpotBlockCreate, type BlockValues } from '../lib/block.schema';
import type { SlotChoice } from '../lib/status';

const SLOTS: readonly SlotChoice[] = ['FULL', 'AM', 'PM'];

export interface BlockSpotsDialogProps {
  establishmentId: string;
  /** Spots to block (id and label shown). */
  spots: readonly { id: string; label: string }[];
  /** Day and slot shown on the map: the defaults of the block. */
  date: string;
  slot: SlotChoice;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}

/**
 * "Blocca postazione" from the operational map (spec 11.3, ADR 0019 C):
 * reason, date range and slot for one or more spots. The server writes one
 * block allocation per spot and day, all or nothing (409 spot_unavailable
 * when one of them is occupied on one of the days).
 */
export function BlockSpotsDialog(props: BlockSpotsDialogProps) {
  const { t } = useTranslation('operations');
  const { spots, open, onOpenChange } = props;
  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={
        spots.length === 1
          ? t('block.titleOne', { label: spots[0]?.label ?? '' })
          : t('block.titleMany', { count: spots.length })
      }
      description={t('block.description')}
    >
      {open && <BlockForm {...props} />}
    </FormDialog>
  );
}

function BlockForm({
  establishmentId,
  spots,
  date,
  slot,
  onOpenChange,
  onDone,
}: BlockSpotsDialogProps) {
  const { t } = useTranslation('operations');
  const id = useId();
  const problemText = useProblemText();
  const create = useCreateSpotBlocks(establishmentId);
  const [failure, setFailure] = useState<string | null>(null);
  // Same key while the body does not change: a double submit blocks once.
  const keyRef = useRef<{ body: string; key: string } | null>(null);
  const form = useForm<BlockValues>({
    resolver: zodResolver(blockSchema),
    defaultValues: blockValues(date, slot),
  });
  const { field: starts } = useController({ control: form.control, name: 'starts_on' });
  const { field: ends } = useController({ control: form.control, name: 'ends_on' });
  const { field: slots } = useController({ control: form.control, name: 'slots' });
  const errors = form.formState.errors;
  const errorText = (key: string | undefined) => (key ? t(key, { defaultValue: key }) : '');

  const onValid = async (values: BlockValues) => {
    setFailure(null);
    const body = toSpotBlockCreate(
      spots.map((s) => s.id),
      values,
    );
    const json = JSON.stringify(body);
    if (keyRef.current?.body !== json) keyRef.current = { body: json, key: newIdempotencyKey() };
    try {
      await create.mutateAsync({ body, idempotencyKey: keyRef.current.key });
      toast.success(
        spots.length === 1
          ? t('block.doneOne', { label: spots[0]?.label ?? '' })
          : t('block.doneMany', { count: spots.length }),
      );
      onDone();
    } catch (err) {
      const mapped = mapProblemToForm(err, form.setError, {
        formKeys: ['starts_on', 'ends_on', 'slots', 'reason'],
      });
      if (mapped.errors.length === 0 || mapped.unmatched.length > 0) {
        setFailure([problemText(err), ...mapped.unmatched].join(' '));
      }
    }
  };

  return (
    <form noValidate onSubmit={(e) => void form.handleSubmit(onValid)(e)} className="space-y-4">
      {failure && (
        <Alert variant="destructive">
          <CircleAlertIcon aria-hidden="true" />
          <AlertDescription>{failure}</AlertDescription>
        </Alert>
      )}
      <div className="space-y-2">
        <p className="text-sm font-medium">{t('block.spots')}</p>
        <ul className="flex flex-wrap gap-1.5" aria-label={t('block.spots')}>
          {spots.map((s) => (
            <li key={s.id}>
              <Badge variant="secondary">{s.label}</Badge>
            </li>
          ))}
        </ul>
      </div>
      <Field data-invalid={!!(errors.starts_on || errors.ends_on) || undefined}>
        <FieldLabel id={`${id}-period-label`}>{t('block.period')}</FieldLabel>
        <DateRangePicker
          aria-labelledby={`${id}-period-label`}
          value={{ from: starts.value, to: ends.value || undefined }}
          onChange={(r) => {
            starts.onChange(r?.from ?? '');
            ends.onChange(r?.to ?? r?.from ?? '');
          }}
          aria-invalid={!!(errors.starts_on || errors.ends_on)}
        />
        <FieldError>{errorText(errors.starts_on?.message ?? errors.ends_on?.message)}</FieldError>
      </Field>
      <Field>
        <FieldLabel>{t('block.slots')}</FieldLabel>
        <SegmentedControl
          label={t('block.slots')}
          value={slots.value}
          onValueChange={(v) => slots.onChange(v as SlotChoice)}
          options={SLOTS.map((s) => ({ value: s, label: t(`slot.${s}`) }))}
        />
      </Field>
      <Field data-invalid={!!errors.reason || undefined}>
        <FieldLabel htmlFor={`${id}-reason`}>
          {t('block.reason')}{' '}
          <span className="font-normal text-muted-foreground">{t('block.optional')}</span>
        </FieldLabel>
        <Input
          id={`${id}-reason`}
          maxLength={200}
          placeholder={t('block.reasonPlaceholder')}
          aria-invalid={!!errors.reason}
          {...form.register('reason')}
        />
        <FieldError>{errorText(errors.reason?.message)}</FieldError>
      </Field>
      <FormActions
        onCancel={() => onOpenChange(false)}
        pending={create.isPending}
        submitLabel={t('block.confirm')}
      />
    </form>
  );
}
