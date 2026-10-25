import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Tabs, TabsContent } from '../tabs';
import { renderWithI18n } from '../../testing';
import {
  ConfirmDialog,
  DataTable,
  DataTableColumnHeader,
  DatePicker,
  DateRangePicker,
  type IsoDateRange,
  I18nTextInput,
  MoneyInput,
  NARROW_QUERY,
  NavTab,
  NavTabs,
  Page,
  PageHeader,
  PercentInput,
  ResponsiveTabsList,
  SectionCard,
  SegmentedControl,
  StatCard,
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
    // The accessible name carries the visible date too (WCAG 2.5.3 Label in Name).
    const trigger = screen.getByRole('button', { name: 'Inizio: 14/07/2026' });
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
    await user.click(screen.getByRole('button', { name: /^Data: / }));
    await user.click(
      await screen.findByRole('button', { name: new RegExp(`12 giugno ${year}`, 'i') }),
    );
    expect(onChange).toHaveBeenCalledWith(`${year}-06-12`);
  });

  it('adds the visible date to an aria-labelledby name', () => {
    renderWithI18n(
      <>
        <span id="lbl">Scadenza</span>
        <DatePicker aria-labelledby="lbl" value="2026-07-14" onChange={vi.fn()} />
      </>,
    );
    expect(screen.getByRole('button', { name: 'Scadenza 14/07/2026' })).toBeInTheDocument();
  });

  it('names an empty field with its label and the placeholder shown', () => {
    renderWithI18n(<DatePicker aria-label="Scadenza" value={undefined} onChange={vi.fn()} />);
    const trigger = screen.getByRole('button', { name: /^Scadenza: / });
    expect(trigger.getAttribute('aria-label')).toContain(trigger.textContent?.trim() ?? '');
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

  interface Pass {
    id: string;
    zone: string;
    spot: string;
    cents: number;
  }
  const passes: Pass[] = [
    { id: 'a1', zone: 'Prima fila', spot: 'A1', cents: 100 },
    { id: 'b1', zone: 'Seconda fila', spot: 'B1', cents: 250 },
    { id: 'a2', zone: 'Prima fila', spot: 'A2', cents: 50 },
  ];
  const passColumns: ColumnDef<Pass>[] = [
    {
      accessorKey: 'spot',
      header: ({ column }) => <DataTableColumnHeader column={column} title="Postazione" />,
      footer: 'Totale',
    },
    {
      accessorKey: 'cents',
      header: 'Saldo',
      meta: { align: 'right' },
      footer: ({ table }) => table.getRowModel().rows.reduce((sum, r) => sum + r.original.cents, 0),
    },
  ];

  it('groups rows by key in order of first appearance, with a heading row', () => {
    renderWithI18n(
      <DataTable
        columns={passColumns}
        data={passes}
        caption="Abbonamenti"
        groupBy={(p) => p.zone}
        renderGroup={(zone, rows) => `${zone} (${rows.length})`}
      />,
    );
    const rows = screen.getAllByRole('row').map((r) => r.textContent);
    expect(rows.slice(1, 6)).toEqual([
      'Prima fila (2)',
      'A1100',
      'A250',
      'Seconda fila (1)',
      'B1250',
    ]);
  });

  it('right-aligns numeric columns and renders the footer totals', () => {
    renderWithI18n(<DataTable columns={passColumns} data={passes} caption="Abbonamenti" />);
    const header = screen.getByRole('columnheader', { name: 'Saldo' });
    expect(header).toHaveClass('text-right', 'tabular-nums');
    expect(screen.getByRole('cell', { name: '250' })).toHaveClass('text-right');
    const footer = screen.getAllByRole('row').at(-1)!;
    expect(footer).toHaveTextContent('Totale400');
  });

  it('delegates search to the server, shows filters and loads more', async () => {
    const user = userEvent.setup();
    const onSearch = vi.fn();
    const onLoadMore = vi.fn();
    renderWithI18n(
      <DataTable
        columns={passColumns}
        data={passes}
        caption="Abbonamenti"
        searchable
        searchValue=""
        onSearchChange={onSearch}
        filters={<button type="button">Stato</button>}
        loadMore={{ hasMore: true, onLoadMore }}
      />,
    );
    await user.type(screen.getByRole('searchbox', { name: 'Filtra…' }), 'z');
    expect(onSearch).toHaveBeenCalledWith('z');
    // Rows are not filtered client-side.
    expect(screen.getByText('B1')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Stato' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Carica altri' }));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it('opens a clickable row with the keyboard', async () => {
    const user = userEvent.setup();
    const onRowClick = vi.fn();
    renderWithI18n(
      <DataTable
        columns={passColumns}
        data={passes}
        caption="Abbonamenti"
        onRowClick={onRowClick}
        getRowLabel={(p) => `Apri ${p.spot}`}
      />,
    );
    screen.getByRole('row', { name: 'Apri B1' }).focus();
    await user.keyboard('{Enter}');
    expect(onRowClick).toHaveBeenCalledWith(passes[1]);
  });

  it('does not open the row when Enter is pressed on a control inside it', async () => {
    const user = userEvent.setup();
    const onRowClick = vi.fn();
    const onEdit = vi.fn();
    const withAction: ColumnDef<Pass>[] = [
      ...passColumns,
      {
        id: 'actions',
        cell: ({ row }) => (
          <button type="button" onClick={(e) => (e.stopPropagation(), onEdit(row.original.id))}>
            Modifica {row.original.spot}
          </button>
        ),
      },
    ];
    renderWithI18n(
      <DataTable
        columns={withAction}
        data={passes}
        caption="Abbonamenti"
        onRowClick={onRowClick}
      />,
    );
    screen.getByRole('button', { name: 'Modifica A1' }).focus();
    await user.keyboard('{Enter}');
    expect(onEdit).toHaveBeenCalledWith('a1');
    expect(onRowClick).not.toHaveBeenCalled();
  });

  it('shows an error in the body and keeps the toolbar', () => {
    renderWithI18n(
      <DataTable
        columns={passColumns}
        data={passes}
        caption="Abbonamenti"
        searchable
        error={<p>Errore di rete</p>}
        loadMore={{ hasMore: true, onLoadMore: () => {} }}
      />,
    );
    expect(screen.getByText('Errore di rete')).toBeInTheDocument();
    expect(screen.getByRole('searchbox', { name: 'Filtra…' })).toBeInTheDocument();
    expect(screen.queryByText('A1')).not.toBeInTheDocument();
    expect(screen.queryByText('Totale')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Carica altri' })).not.toBeInTheDocument();
  });
});

describe('Page / SectionCard / StatCard / NavTabs', () => {
  it('labels the page section by its title and renders sections and KPIs', () => {
    renderWithI18n(
      <Page
        title="Cassa"
        description="Sessione corrente"
        actions={<button type="button">Incassa</button>}
      >
        <StatCard label="Totale incassi" value="157,80 €" tone="danger" hint="oggi" />
        <SectionCard
          title="Incassi per metodo"
          description="Per metodo"
          actions={<button type="button">Esporta</button>}
        >
          <p>contenuto</p>
        </SectionCard>
      </Page>,
    );
    expect(screen.getByRole('region', { name: 'Cassa' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Cassa' })).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { level: 2, name: 'Incassi per metodo' }),
    ).toBeInTheDocument();
    expect(screen.getByText('157,80 €')).toHaveClass('text-destructive', 'tabular-nums');
    expect(screen.getByRole('button', { name: 'Esporta' })).toBeInTheDocument();
  });

  const navOptions = [
    { value: '/a', label: 'Listino' },
    { value: '/b', label: 'Matrice' },
  ];
  function Nav({ onValueChange = () => {} }: { onValueChange?: (v: string) => void }) {
    return (
      <NavTabs
        label="Strumenti prezzi"
        options={navOptions}
        value="/a"
        onValueChange={onValueChange}
      >
        <NavTab active>
          <a href="/a">Listino</a>
        </NavTab>
        <NavTab active={false}>
          <a href="/b">Matrice</a>
        </NavTab>
      </NavTabs>
    );
  }

  it('marks the active route tab', () => {
    renderWithI18n(<Nav />);
    const nav = screen.getByRole('navigation', { name: 'Strumenti prezzi' });
    expect(nav).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Listino' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Matrice' })).toHaveAttribute('data-state', 'inactive');
  });

  describe('on a narrow screen', () => {
    const original = window.matchMedia;
    beforeEach(() => {
      window.matchMedia = ((query: string) => ({
        matches: query === NARROW_QUERY,
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      })) as typeof window.matchMedia;
    });
    afterEach(() => {
      window.matchMedia = original;
    });

    it('route tabs become a Select that navigates', async () => {
      const user = userEvent.setup();
      const onValueChange = vi.fn();
      renderWithI18n(<Nav onValueChange={onValueChange} />);
      expect(screen.queryByRole('link')).not.toBeInTheDocument();
      const select = screen.getByRole('combobox', { name: 'Strumenti prezzi' });
      expect(select).toHaveTextContent('Listino');
      await user.click(select);
      await user.click(await screen.findByRole('option', { name: 'Matrice' }));
      expect(onValueChange).toHaveBeenCalledWith('/b');
    });

    it('tabs become a Select driving the same Tabs value', async () => {
      const user = userEvent.setup();
      function Harness() {
        const [tab, setTab] = useState('general');
        const options = [
          { value: 'general', label: 'Generale' },
          { value: 'pricing', label: 'Prezzi e IVA' },
        ];
        return (
          <Tabs value={tab} onValueChange={setTab}>
            <ResponsiveTabsList
              label="Sezioni"
              value={tab}
              onValueChange={setTab}
              options={options}
            />
            <TabsContent value="general">Pannello generale</TabsContent>
            <TabsContent value="pricing">Pannello prezzi</TabsContent>
          </Tabs>
        );
      }
      renderWithI18n(<Harness />);
      expect(screen.queryByRole('tablist')).not.toBeInTheDocument();
      expect(screen.getByText('Pannello generale')).toBeInTheDocument();
      await user.click(screen.getByRole('combobox', { name: 'Sezioni' }));
      await user.click(await screen.findByRole('option', { name: 'Prezzi e IVA' }));
      expect(await screen.findByText('Pannello prezzi')).toBeInTheDocument();
    });

    it('collapses a long segmented control, keeps a short one', () => {
      const options = [
        { value: 'FULL', label: 'Giorno intero' },
        { value: 'AM', label: 'Mattina' },
        { value: 'PM', label: 'Pomeriggio' },
      ];
      renderWithI18n(
        <>
          <SegmentedControl
            collapseOnNarrow
            label="Fasce"
            value="AM"
            onValueChange={() => {}}
            options={options}
          />
          <SegmentedControl label="Periodo" value="AM" onValueChange={() => {}} options={options} />
        </>,
      );
      expect(screen.getByRole('combobox', { name: 'Fasce' })).toHaveTextContent('Mattina');
      expect(screen.getByRole('radiogroup', { name: 'Periodo' })).toBeInTheDocument();
    });
  });

  it('renders tabs and a segmented control on a wide screen', async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    renderWithI18n(
      <>
        <Tabs value="general">
          <ResponsiveTabsList
            label="Sezioni"
            value="general"
            onValueChange={() => {}}
            options={[
              { value: 'general', label: 'Generale' },
              { value: 'pricing', label: 'Prezzi e IVA' },
            ]}
          />
        </Tabs>
        <SegmentedControl
          collapseOnNarrow
          label="Fasce"
          value="FULL"
          onValueChange={onValueChange}
          options={[
            { value: 'FULL', label: 'Giorno intero' },
            { value: 'AM', label: 'Mattina' },
          ]}
        />
      </>,
    );
    expect(screen.getByRole('tablist', { name: 'Sezioni' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Generale' })).toHaveAttribute('aria-selected', 'true');
    await user.click(screen.getByRole('radio', { name: 'Mattina' }));
    expect(onValueChange).toHaveBeenCalledWith('AM');
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

describe('DateRangePicker', () => {
  function Harness({
    initial,
    onChange,
  }: {
    initial?: IsoDateRange;
    onChange: (v?: IsoDateRange) => void;
  }) {
    const [v, setV] = useState<IsoDateRange | undefined>(initial);
    return (
      <DateRangePicker
        aria-label="Periodo"
        value={v}
        min="2026-01-01"
        max="2026-12-31"
        onChange={(next) => {
          setV(next);
          onChange(next);
        }}
      />
    );
  }

  it('starts a new range on the first click and ends it on the second', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithI18n(
      <Harness initial={{ from: '2026-01-01', to: '2026-05-31' }} onChange={onChange} />,
    );
    await user.click(screen.getByRole('button', { name: /^Periodo: / }));
    await user.click(await screen.findByRole('button', { name: /10 gennaio 2026/i }));
    expect(onChange).toHaveBeenLastCalledWith({ from: '2026-01-10' });
    await user.click(screen.getByRole('button', { name: /20 febbraio 2026/i }));
    expect(onChange).toHaveBeenLastCalledWith({ from: '2026-01-10', to: '2026-02-20' });
  });

  it('accepts typed dates and rejects invalid or out-of-bounds ones', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    renderWithI18n(
      <Harness initial={{ from: '2026-01-01', to: '2026-01-31' }} onChange={onChange} />,
    );
    await user.click(screen.getByRole('button', { name: /^Periodo: / }));
    const from = await screen.findByLabelText('Dal');
    const to = screen.getByLabelText('Al');
    expect(from).toHaveValue('01/01/2026');
    await user.clear(to);
    await user.type(to, '15/8/2026{Enter}');
    expect(onChange).toHaveBeenLastCalledWith({ from: '2026-01-01', to: '2026-08-15' });
    expect(to).toHaveValue('15/08/2026');
    onChange.mockClear();
    await user.clear(from);
    await user.type(from, '31/02/2026{Enter}');
    expect(from).toHaveAttribute('aria-invalid', 'true');
    await user.clear(from);
    await user.type(from, '01/01/2027{Enter}');
    expect(from).toHaveAttribute('aria-invalid', 'true');
    expect(onChange).not.toHaveBeenCalled();
  });
});
