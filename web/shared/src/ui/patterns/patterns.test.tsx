import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithI18n } from '../../testing';
import {
  ConfirmDialog,
  DataTable,
  DataTableColumnHeader,
  DatePicker,
  I18nTextInput,
  MoneyInput,
  PageHeader,
  PercentInput,
  ThemeProvider,
  ThemeToggle,
  type ColumnDef,
} from './index';

function MoneyHarness({
  initial,
  onValue,
}: {
  initial: number | null;
  onValue: (v: number | null) => void;
}) {
  const [v, setV] = useState(initial);
  return (
    <MoneyInput
      aria-label="Prezzo"
      value={v}
      onChange={(n) => {
        setV(n);
        onValue(n);
      }}
    />
  );
}

describe('MoneyInput / PercentInput', () => {
  it('shows Italian formatting and emits integer cents', async () => {
    const user = userEvent.setup();
    const onValue = vi.fn();
    renderWithI18n(<MoneyHarness initial={123450} onValue={onValue} />);
    const input = screen.getByLabelText('Prezzo');
    expect(input).toHaveValue('1.234,50');
    await user.clear(input);
    await user.type(input, '19,99');
    expect(onValue).toHaveBeenLastCalledWith(1999);
    await user.tab();
    expect(input).toHaveValue('19,99');
    await user.clear(input);
    await user.type(input, '12,345');
    expect(onValue).toHaveBeenLastCalledWith(null);
    expect(input).toHaveAttribute('aria-invalid', 'true');
    // An unparsable entry stays visible after blur so it can be corrected.
    await user.tab();
    expect(input).toHaveValue('12,345');
    expect(input).toHaveAttribute('aria-invalid', 'true');
  });

  it('reports the raw text, applies a value received while focused on blur and names the unit', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = renderWithI18n(
      <MoneyInput aria-label="Prezzo" value={1000} onChange={onChange} />,
    );
    const input = screen.getByLabelText('Prezzo');
    expect(input).toHaveAccessibleDescription('euro');
    await user.clear(input);
    await user.type(input, 'abc');
    expect(onChange).toHaveBeenLastCalledWith(null, 'abc');
    // A reset while the field is focused is applied when the user leaves it.
    rerender(<MoneyInput aria-label="Prezzo" value={2500} onChange={onChange} />);
    expect(input).toHaveValue('abc');
    await user.tab();
    expect(input).toHaveValue('25,00');
  });

  it('binds percentages to basis points', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithI18n(<PercentInput aria-label="IVA" value={2200} onChange={onChange} />);
    const input = screen.getByLabelText('IVA');
    expect(input).toHaveValue('22');
    await user.clear(input);
    await user.type(input, '5,5');
    expect(onChange).toHaveBeenLastCalledWith(550, '5,5');
  });
});

describe('DatePicker', () => {
  it('shows dd/MM/yyyy and picks a day as ISO', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithI18n(<DatePicker aria-label="Inizio" value="2026-07-14" onChange={onChange} />);
    const trigger = screen.getByRole('button', { name: 'Inizio' });
    expect(trigger).toHaveTextContent('14/07/2026');
    await user.click(trigger);
    await user.click(await screen.findByRole('button', { name: /20 luglio 2026/i }));
    expect(onChange).toHaveBeenCalledWith('2026-07-20');
  });

  it('opens on a date years ahead and can pick it (dropdown range)', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const year = new Date().getFullYear() + 3;
    renderWithI18n(<DatePicker aria-label="Data" value={`${year}-06-10`} onChange={onChange} />);
    await user.click(screen.getByRole('button', { name: 'Data' }));
    await user.click(
      await screen.findByRole('button', { name: new RegExp(`12 giugno ${year}`, 'i') }),
    );
    expect(onChange).toHaveBeenCalledWith(`${year}-06-12`);
  });
});

interface Row {
  id: string;
  name: string;
  price: number;
}
const columns: ColumnDef<Row>[] = [
  {
    accessorKey: 'name',
    header: ({ column }) => <DataTableColumnHeader column={column} title="Nome" />,
    meta: { label: 'Nome' },
  },
  { accessorKey: 'price', header: 'Prezzo', meta: { label: 'Prezzo' } },
];

describe('DataTable', () => {
  const data: Row[] = Array.from({ length: 25 }, (_, i) => ({
    id: `r${i}`,
    name: `Ombrellone ${String(i + 1).padStart(2, '0')}`,
    price: i,
  }));

  it('filters, paginates and shows the empty state', async () => {
    const user = userEvent.setup();
    renderWithI18n(
      <DataTable columns={columns} data={data} caption="Postazioni" searchable pageSize={10} />,
    );
    expect(screen.getByRole('table', { name: 'Postazioni' })).toBeInTheDocument();
    expect(screen.getAllByRole('row')).toHaveLength(11);
    expect(screen.getByText('Pagina 1 di 3')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Vai alla pagina successiva' }));
    expect(screen.getByText('Ombrellone 11')).toBeInTheDocument();
    await user.type(screen.getByRole('searchbox', { name: 'Filtra…' }), 'nessuno');
    expect(screen.getByText('Nessun risultato.')).toBeInTheDocument();
  });

  it('sorts from the column header menu', async () => {
    const user = userEvent.setup();
    renderWithI18n(<DataTable columns={columns} data={data.slice(0, 3)} caption="Postazioni" />);
    await user.click(screen.getByRole('button', { name: 'Ordina per Nome' }));
    await user.click(await screen.findByRole('menuitem', { name: 'Decrescente' }));
    const firstCell = screen.getAllByRole('row')[1];
    expect(firstCell).toHaveTextContent('Ombrellone 03');
  });
});

describe('ConfirmDialog', () => {
  it('waits for async confirmation before closing', async () => {
    const user = userEvent.setup();
    let resolve: () => void = () => {};
    const onConfirm = vi.fn(() => new Promise<void>((r) => (resolve = r)));
    renderWithI18n(
      <ConfirmDialog
        title="Eliminare la zona?"
        destructive
        confirmLabel="Elimina"
        onConfirm={onConfirm}
        trigger={<button type="button">Apri</button>}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Apri' }));
    await user.click(await screen.findByRole('button', { name: 'Elimina' }));
    expect(onConfirm).toHaveBeenCalled();
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
    resolve();
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
  });
});

describe('I18nTextInput', () => {
  it('edits one locale per tab and flags missing translations', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithI18n(
      <I18nTextInput
        id="name"
        value={{ it: 'Ombrellone' }}
        onChange={onChange}
        locales={['it', 'en']}
      />,
    );
    expect(screen.getByRole('tab', { name: /en.*da tradurre/i })).toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: /en/i }));
    await user.type(screen.getByRole('textbox', { name: 'English' }), 'U');
    expect(onChange).toHaveBeenLastCalledWith({ it: 'Ombrellone', en: 'U' });
  });
});

describe('ThemeProvider', () => {
  it('applies the dark class from the toggle', async () => {
    const user = userEvent.setup();
    renderWithI18n(
      <ThemeProvider defaultTheme="light">
        <PageHeader title="Prova" actions={<ThemeToggle />} />
      </ThemeProvider>,
    );
    expect(screen.getByRole('heading', { level: 1, name: 'Prova' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Cambia tema' }));
    await user.click(await screen.findByRole('menuitemradio', { name: 'Scuro' }));
    expect(document.documentElement).toHaveClass('dark');
    document.documentElement.classList.remove('dark');
    window.localStorage.clear();
  });
});
