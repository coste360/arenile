import { formatIsoDate } from '@arenile/shared/lib/dates';
import { formatCents } from '@arenile/shared/lib/money';
import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Spinner } from '@arenile/shared/ui/spinner';
import { FormDialog, SectionCard, StatusBadge } from '@arenile/shared/ui/patterns';
import { Link } from '@tanstack/react-router';
import { BadgeCheckIcon, HandshakeIcon, PencilIcon } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../../establishment';
import { useVerifyBookingRequirement } from '../../partners/api';
import { describeM8Problem } from '../../partners/lib';
import { usePatchBooking, type BookingChangeResult, type BookingDetail } from '../api';
import { ChangeSummary } from '../detail/ChangeDialogs';
import type { ProblemText } from '../problem';
import { ProblemAlert } from '../ProblemAlert';
import type { EstablishmentSpots } from '../spots';
import { BookingAgreementFields } from './BookingAgreementFields';
import { changesPrice, fromDetail, toPatch, type BookingAgreementValue } from './model';
import { useAgreementOptions } from './useAgreementOptions';

/**
 * "Convenzione e ospiti" of the booking detail (spec 30.2.3, 30.2.4, 30.3):
 * agreement and partner badges, share charged to the partner, group and
 * party, the requirement check ("verificato": date and operator) and the
 * edit dialog, which re-quotes a change of agreement with a dry run (spec
 * 10.3: difference to collect or refund) before applying it.
 */
export function BookingAgreementCard({
  establishmentId,
  booking: b,
  canWrite,
  inventory,
  onReload,
}: {
  establishmentId: string;
  booking: BookingDetail;
  canWrite: boolean;
  inventory: EstablishmentSpots;
  onReload: () => void;
}) {
  const { t } = useTranslation('partners');
  const fmt = useInstantFormat();
  const verify = useVerifyBookingRequirement(establishmentId);
  const [editing, setEditing] = useState(false);
  const requirement = b.agreement?.requirement;
  const verification = b.requirement_verification;
  const party = (b.party_adults ?? 0) + (b.party_children ?? 0);
  const share = b.partner_share;

  const markVerified = async () => {
    try {
      await verify.mutateAsync({ bookingId: b.id, version: b.version });
      toast.success(t('booking.verifiedDone'));
    } catch (err) {
      toast.error(describeM8Problem(err, t).title);
    }
  };

  return (
    <SectionCard
      icon={HandshakeIcon}
      title={t('booking.section')}
      actions={
        canWrite && (
          <Button size="sm" variant="outline" onClick={() => setEditing(true)}>
            <PencilIcon aria-hidden="true" />
            {t('common.actions.edit', { ns: 'translation' })}
          </Button>
        )
      }
    >
      <dl className="space-y-4 text-sm">
        <Item label={t('booking.agreement')}>
          {b.agreement ? (
            <span className="flex flex-wrap items-center gap-1.5">
              <span className="font-medium text-foreground">{b.agreement.name}</span>
              {b.agreement.code && (
                <Badge variant="outline" className="font-mono">
                  {b.agreement.code}
                </Badge>
              )}
            </span>
          ) : (
            t('booking.noAgreement')
          )}
        </Item>
        {requirement && (
          <Item label={t('booking.requirement')}>
            <span className="space-y-2">
              <span className="block">{requirement}</span>
              {verification ? (
                <StatusBadge tone="success">
                  {t('booking.verifiedBy', {
                    at: fmt.dateTime(verification.verified_at),
                    name: verification.verified_by_name ?? '',
                  })}
                </StatusBadge>
              ) : (
                <span className="flex flex-wrap items-center gap-2">
                  <StatusBadge tone="warning">{t('booking.toVerify')}</StatusBadge>
                  {canWrite && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => void markVerified()}
                      disabled={verify.isPending}
                    >
                      {verify.isPending ? <Spinner /> : <BadgeCheckIcon aria-hidden="true" />}
                      {t('booking.markVerified')}
                    </Button>
                  )}
                </span>
              )}
            </span>
          </Item>
        )}
        <Item label={t('booking.partner')}>
          {b.partner ? (
            <span className="block space-y-1">
              <Link
                to="/e/$establishmentId/convenzioni/partner/$partnerId"
                params={{ establishmentId, partnerId: b.partner.id }}
                className="font-medium text-primary-on-surface underline-offset-4 hover:underline"
              >
                {b.partner.name}
              </Link>
              {share && share.type !== 'none' && (
                <span className="block">
                  {t(`booking.shareSummary.${share.type}`, {
                    percent: share.percent ?? '',
                    amount: formatCents(b.partner_amount ?? share.amount ?? 0),
                  })}
                </span>
              )}
              {b.partner_guest_stay && (
                <span className="block">
                  {t('booking.guestStayValue', {
                    from: formatIsoDate(b.partner_guest_stay.from),
                    to: formatIsoDate(b.partner_guest_stay.to),
                  })}
                </span>
              )}
            </span>
          ) : (
            b.group_origin || '—'
          )}
        </Item>
        {(b.group_name || party > 0) && (
          <Item label={t('booking.group')}>
            {[
              b.group_name,
              party > 0 &&
                t('booking.partyValue', {
                  adults: b.party_adults ?? 0,
                  children: b.party_children ?? 0,
                }),
            ]
              .filter(Boolean)
              .join(' · ')}
          </Item>
        )}
        {!!b.guest_names?.length && (
          <Item label={t('booking.guestNames')}>{b.guest_names.join(', ')}</Item>
        )}
      </dl>
      <FormDialog
        open={editing}
        onOpenChange={setEditing}
        title={t('booking.editTitle', { code: b.code })}
        description={t('booking.editDescription')}
        wide
      >
        {editing && (
          <EditForm
            establishmentId={establishmentId}
            booking={b}
            inventory={inventory}
            onReload={onReload}
            onDone={() => setEditing(false)}
          />
        )}
      </FormDialog>
    </SectionCard>
  );
}

function Item({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="font-medium">{label}</dt>
      <dd className="text-muted-foreground">{children}</dd>
    </div>
  );
}

function EditForm({
  establishmentId,
  booking,
  inventory,
  onReload,
  onDone,
}: {
  establishmentId: string;
  booking: BookingDetail;
  inventory: EstablishmentSpots;
  onReload: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation('partners');
  const { t: tb } = useTranslation('bookings');
  const patch = usePatchBooking(establishmentId);
  const [value, setValue] = useState<BookingAgreementValue>(() => fromDetail(booking));
  const [preview, setPreview] = useState<BookingChangeResult | null>(null);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const options = useAgreementOptions(establishmentId, value.agreementId ?? '', value.partnerId);
  const body = toPatch(value, booking);
  const unchanged = Object.keys(body).length === 0;
  const needsPreview = changesPrice(body);

  const run = async (dryRun: boolean) => {
    setFailure(null);
    try {
      const result = await patch.mutateAsync({
        bookingId: booking.id,
        version: booking.version,
        patch: body,
        dryRun,
      });
      if (dryRun) setPreview(result);
      else {
        toast.success(t('booking.saved'));
        onDone();
      }
    } catch (err) {
      setFailure(
        describeM8Problem(err, tb, {
          resourceLabel: (g) => inventory.spotById.get(g.resourceId)?.label,
        }),
      );
    }
  };

  return (
    <div className="space-y-4">
      <ProblemAlert problem={failure} onReload={onReload} />
      <BookingAgreementFields
        value={value}
        onChange={(next) => {
          setValue(next);
          setPreview(null);
        }}
        options={options}
      />
      {preview && <ChangeSummary result={preview} inventory={inventory} />}
      <DialogFooter className="border-t pt-4">
        <Button variant="outline" onClick={onDone}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
        {needsPreview && !preview ? (
          <Button onClick={() => void run(true)} disabled={patch.isPending || unchanged}>
            {patch.isPending && <Spinner />}
            {tb('change.requote')}
          </Button>
        ) : (
          <Button
            onClick={() => void run(false)}
            disabled={patch.isPending || unchanged || (!!preview && preview.conflicts.length > 0)}
          >
            {patch.isPending && <Spinner />}
            {needsPreview ? tb('change.apply') : t('common.actions.save', { ns: 'translation' })}
          </Button>
        )}
      </DialogFooter>
    </div>
  );
}
