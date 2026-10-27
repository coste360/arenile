import { Badge } from '@arenile/shared/ui/badge';
import { Button } from '@arenile/shared/ui/button';
import { Input } from '@arenile/shared/ui/input';
import { Label } from '@arenile/shared/ui/label';
import { FormDialog, MultiSelect, OptionSelect, SectionCard } from '@arenile/shared/ui/patterns';
import { DialogFooter } from '@arenile/shared/ui/dialog';
import { Spinner } from '@arenile/shared/ui/spinner';
import { BadgeCheckIcon, HandshakeIcon, PencilIcon } from 'lucide-react';
import { useId, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { useInstantFormat } from '../../../establishment';
import { deskAgreements } from '../../bookings/agreement/model';
import type { ProblemText } from '../../bookings/problem';
import { ProblemAlert } from '../../bookings/ProblemAlert';
import {
  customerCategories,
  useCustomerVerifications,
  useVerifyCustomerRequirement,
  type CustomerCategory,
} from '../../partners/api';
import { describeM8Problem } from '../../partners/lib';
import { agreements as agreementsResource, type Agreement } from '../../pricing/api';
import { useUpdateCustomer, type Customer } from '../api';
import { withAgreement } from './model';

const NONE = '__none';

/**
 * "Convenzione e categorie" of the customer card (spec 30.2.2, 30.2.3): the
 * default agreement proposed on every booking, the customer categories
 * (residents, over 65...) and the requirement checks (date and operator,
 * never a copy or the number of a document, spec 23).
 */
export function CustomerAgreementCard({
  establishmentId,
  customer: c,
  canWrite,
}: {
  establishmentId: string;
  customer: Customer;
  canWrite: boolean;
}) {
  const { t } = useTranslation('partners');
  const fmt = useInstantFormat();
  const agreementList = agreementsResource.useList(establishmentId);
  const categoryList = customerCategories.useList(establishmentId);
  const verifications = useCustomerVerifications(establishmentId, c.id);
  const [editing, setEditing] = useState(false);
  const [verifying, setVerifying] = useState(false);
  const agreement = (agreementList.data ?? []).find((a) => a.id === c.agreement_id);
  const categories = (categoryList.data ?? []).filter((x) => c.category_ids?.includes(x.id));
  const requirements = [
    ...(agreement?.requirement
      ? [{ kind: 'agreement' as const, id: agreement.id, text: agreement.requirement }]
      : []),
    ...categories
      .filter((x) => x.requirement)
      .map((x) => ({ kind: 'category' as const, id: x.id, text: x.requirement })),
  ];
  const items = verifications.data ?? [];

  return (
    <SectionCard
      icon={HandshakeIcon}
      title={t('customer.title')}
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
        <Item label={t('customer.agreement')}>
          {agreement ? (
            <span className="flex flex-wrap items-center gap-1.5">
              <span className="font-medium text-foreground">{agreement.name}</span>
              {agreement.code && (
                <Badge variant="outline" className="font-mono">
                  {agreement.code}
                </Badge>
              )}
            </span>
          ) : (
            t('booking.noAgreement')
          )}
        </Item>
        <Item label={t('customer.categories')}>
          {categories.length ? (
            <span className="flex flex-wrap gap-1">
              {categories.map((x) => (
                <Badge key={x.id} variant="outline">
                  {x.name}
                </Badge>
              ))}
            </span>
          ) : (
            '—'
          )}
        </Item>
        <Item label={t('customer.verifications')}>
          {items.length ? (
            <ul className="space-y-1">
              {items.slice(0, 5).map((v) => (
                <li key={v.id}>
                  {t('customer.verificationLine', {
                    requirement: v.requirement,
                    at: fmt.date(v.verified_at),
                    name: v.verified_by_name ?? '',
                  })}
                </li>
              ))}
            </ul>
          ) : (
            t('customer.noVerifications')
          )}
          {canWrite && requirements.length > 0 && (
            <Button size="sm" variant="outline" className="mt-2" onClick={() => setVerifying(true)}>
              <BadgeCheckIcon aria-hidden="true" />
              {t('customer.verify')}
            </Button>
          )}
        </Item>
      </dl>
      <FormDialog
        open={editing}
        onOpenChange={setEditing}
        title={t('customer.editTitle')}
        description={t('customer.editDescription')}
      >
        {editing && (
          <EditForm
            establishmentId={establishmentId}
            customer={c}
            agreements={deskAgreements(agreementList.data ?? [], c.agreement_id)}
            categories={(categoryList.data ?? []).filter(
              (x) => x.active || c.category_ids?.includes(x.id),
            )}
            onDone={() => setEditing(false)}
          />
        )}
      </FormDialog>
      <FormDialog
        open={verifying}
        onOpenChange={setVerifying}
        title={t('customer.verifyTitle')}
        description={t('customer.verifyDescription')}
      >
        {verifying && (
          <VerifyForm
            establishmentId={establishmentId}
            customerId={c.id}
            requirements={requirements}
            onDone={() => setVerifying(false)}
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
  customer,
  agreements,
  categories,
  onDone,
}: {
  establishmentId: string;
  customer: Customer;
  agreements: Agreement[];
  categories: CustomerCategory[];
  onDone: () => void;
}) {
  const { t } = useTranslation('partners');
  const id = useId();
  const update = useUpdateCustomer(establishmentId);
  const [agreementId, setAgreementId] = useState(customer.agreement_id ?? '');
  const [categoryIds, setCategoryIds] = useState<string[]>(customer.category_ids ?? []);
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const save = async () => {
    setFailure(null);
    try {
      await update.mutateAsync({
        id: customer.id,
        version: customer.version,
        input: withAgreement(customer, agreementId, categoryIds),
      });
      toast.success(t('customer.saved'));
      onDone();
    } catch (err) {
      setFailure(describeM8Problem(err, t));
    }
  };
  return (
    <div className="space-y-4">
      <ProblemAlert problem={failure} />
      <div className="space-y-2">
        <Label htmlFor={`${id}-agreement`}>{t('customer.agreement')}</Label>
        <OptionSelect
          id={`${id}-agreement`}
          value={agreementId || NONE}
          onChange={(v) => setAgreementId(v === NONE ? '' : v)}
          options={[
            { value: NONE, label: t('booking.noAgreement') },
            ...agreements.map((a) => ({ value: a.id, label: a.name })),
          ]}
        />
        <p className="text-xs text-muted-foreground">{t('customer.agreementHint')}</p>
      </div>
      <div className="space-y-2">
        <Label id={`${id}-categories`}>{t('customer.categories')}</Label>
        <MultiSelect
          aria-labelledby={`${id}-categories`}
          value={categoryIds}
          onChange={setCategoryIds}
          options={categories.map((x) => ({ value: x.id, label: x.name }))}
          placeholder={t('customer.noCategory')}
        />
      </div>
      <DialogFooter className="border-t pt-4">
        <Button variant="outline" onClick={onDone}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
        <Button onClick={() => void save()} disabled={update.isPending}>
          {update.isPending && <Spinner />}
          {t('common.actions.save', { ns: 'translation' })}
        </Button>
      </DialogFooter>
    </div>
  );
}

function VerifyForm({
  establishmentId,
  customerId,
  requirements,
  onDone,
}: {
  establishmentId: string;
  customerId: string;
  requirements: { kind: 'agreement' | 'category'; id: string; text: string }[];
  onDone: () => void;
}) {
  const { t } = useTranslation('partners');
  const id = useId();
  const verify = useVerifyCustomerRequirement(establishmentId);
  const [choice, setChoice] = useState(
    requirements[0] ? `${requirements[0].kind}:${requirements[0].id}` : '',
  );
  const [note, setNote] = useState('');
  const [failure, setFailure] = useState<ProblemText | null>(null);
  const save = async () => {
    const [kind, target] = choice.split(':') as ['agreement' | 'category', string];
    setFailure(null);
    try {
      await verify.mutateAsync({
        customerId,
        input: {
          ...(kind === 'agreement' ? { agreement_id: target } : { customer_category_id: target }),
          ...(note.trim() ? { note: note.trim() } : {}),
        },
      });
      toast.success(t('booking.verifiedDone'));
      onDone();
    } catch (err) {
      setFailure(describeM8Problem(err, t));
    }
  };
  return (
    <div className="space-y-4">
      <ProblemAlert problem={failure} />
      <div className="space-y-2">
        <Label htmlFor={`${id}-requirement`}>{t('booking.requirement')}</Label>
        <OptionSelect
          id={`${id}-requirement`}
          value={choice}
          onChange={setChoice}
          options={requirements.map((r) => ({ value: `${r.kind}:${r.id}`, label: r.text }))}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor={`${id}-note`}>{t('customer.note')}</Label>
        <Input
          id={`${id}-note`}
          maxLength={120}
          autoComplete="off"
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
        <p className="text-xs text-muted-foreground">{t('customer.noteHint')}</p>
      </div>
      <DialogFooter className="border-t pt-4">
        <Button variant="outline" onClick={onDone}>
          {t('common.actions.cancel', { ns: 'translation' })}
        </Button>
        <Button onClick={() => void save()} disabled={verify.isPending || !choice}>
          {verify.isPending && <Spinner />}
          {t('booking.markVerified')}
        </Button>
      </DialogFooter>
    </div>
  );
}
