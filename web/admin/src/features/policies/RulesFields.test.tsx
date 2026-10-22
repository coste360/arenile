import { Form } from '@arenile/shared/ui';
import { zodResolver } from '@hookform/resolvers/zod';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useForm } from 'react-hook-form';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import {
  defaultPolicySchema,
  rulesValues,
  toRules,
  type DefaultPolicyValues,
  type PolicyRules,
} from './policy.schema';
import { RulesFields } from './RulesFields';

function Harness({ rules, onSave }: { rules: PolicyRules; onSave: (r: PolicyRules) => void }) {
  const form = useForm<DefaultPolicyValues>({
    resolver: zodResolver(defaultPolicySchema),
    defaultValues: rulesValues(rules),
  });
  return (
    <Form {...form}>
      <form noValidate onSubmit={form.handleSubmit((v) => onSave(toRules(v)))}>
        <RulesFields control={form.control} />
        <button type="submit">salva</button>
      </form>
    </Form>
  );
}

function setup(rules: PolicyRules) {
  const saved: PolicyRules[] = [];
  const user = userEvent.setup();
  render(
    <I18nextProvider i18n={createAdminTestI18n()}>
      <Harness rules={rules} onSave={(r) => saved.push(r)} />
    </I18nextProvider>,
  );
  return { user, saved, tiers: () => screen.queryAllByTestId('cancellation-tier') };
}

const daysOf = (row: HTMLElement) =>
  (within(row).getByLabelText('Fino a giorni prima') as HTMLInputElement).value;

describe('cancellation tiers editor', () => {
  it('adds, edits, removes and sorts tiers, saving them furthest-first', async () => {
    const { user, saved, tiers } = setup({});
    await user.click(screen.getByLabelText('Definisci le fasce di rimborso'));
    expect(
      screen.getByText('Nessuna fascia: le cancellazioni non sono rimborsate.'),
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Aggiungi fascia' }));
    await user.click(screen.getByRole('button', { name: 'Aggiungi fascia' }));
    await user.click(screen.getByRole('button', { name: 'Aggiungi fascia' }));
    expect(tiers()).toHaveLength(3);
    // Suggested rows halve days and refund of the closest tier: 7/100, 3/50, 1/25.
    expect(tiers().map(daysOf)).toEqual(['7', '3', '1']);

    const third = tiers()[2] as HTMLElement;
    const days = within(third).getByLabelText('Fino a giorni prima');
    await user.clear(days);
    await user.type(days, '30');
    await user.click(within(third).getByRole('combobox', { name: 'Come' }));
    await user.click(await screen.findByRole('option', { name: 'Credito' }));

    await user.click(screen.getByRole('button', { name: 'Rimuovi la fascia 2' }));
    expect(tiers().map(daysOf)).toEqual(['7', '30']);

    await user.click(screen.getByRole('button', { name: 'Ordina per giorni' }));
    expect(tiers().map(daysOf)).toEqual(['30', '7']);

    await user.click(screen.getByRole('button', { name: 'Sposta giù la fascia 1' }));
    expect(tiers().map(daysOf)).toEqual(['7', '30']);
    expect(screen.getByRole('button', { name: 'Sposta su la fascia 1' })).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'salva' }));
    expect(saved).toEqual([
      {
        cancellation: [
          { until_days_before: 30, refund_percent: 25, as: 'credit' },
          { until_days_before: 7, refund_percent: 100, as: 'refund' },
        ],
      },
    ]);
  });

  it('flags duplicate days and out-of-range refunds', async () => {
    const { user, saved } = setup({
      cancellation: [
        { until_days_before: 7, refund_percent: 100, as: 'refund' },
        { until_days_before: 7, refund_percent: 150, as: 'refund' },
      ],
    });
    await user.click(screen.getByRole('button', { name: 'salva' }));
    expect(saved).toHaveLength(0);
    expect(await screen.findByText('Esiste già una fascia con questi giorni.')).toBeInTheDocument();
    const refund = screen.getAllByLabelText('Rimborso (%)')[1] as HTMLInputElement;
    expect(refund).toHaveAttribute('aria-invalid', 'true');
  });

  it('keeps an explicit empty list (no refund) distinct from inheriting', async () => {
    const { user, saved } = setup({ cancellation: [] });
    await user.click(screen.getByRole('button', { name: 'salva' }));
    expect(saved).toEqual([{ cancellation: [] }]);
  });
});

describe('money fields', () => {
  it('rejects an unreadable modification fee instead of silently dropping it', async () => {
    const { user, saved } = setup({});
    const fee = screen.getByLabelText('Costo modifica (€)');
    await user.type(fee, 'abc');
    await user.click(screen.getByRole('button', { name: 'salva' }));
    expect(saved).toHaveLength(0);
    expect(
      await screen.findByText('Inserisci un importo valido, ad esempio 25 o 25,50'),
    ).toBeInTheDocument();
    expect(fee).toHaveAttribute('aria-invalid', 'true');
  });
});
