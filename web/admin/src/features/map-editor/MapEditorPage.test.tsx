import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
  problem,
  renderApp,
  type Handler,
} from '../configuration/testing';

const API = `/api/admin/v1/establishments/${EST_ID}`;
const ts = { created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' };
const MAP = {
  id: 'm1',
  name: { it: 'Spiaggia' },
  width: 400,
  height: 300,
  grid_size: 10,
  sea_side: 'bottom',
  sort_order: 0,
  spot_count: 2,
  version: 3,
  ...ts,
};
const spot = (id: string, label: string, x: number, y = 200) => ({
  id,
  map_id: 'm1',
  zone_id: 'z1',
  label,
  row_label: 'A',
  number: Number(label.slice(1)),
  kind: 'umbrella',
  x,
  y,
  rotation: 0,
  shape: { type: 'circle', w: 20, h: 20 },
  capacity: { umbrella: 1 },
  max_extra: {},
  attributes: [],
  active: true,
});
const DOC = { map: MAP, spots: [spot('s1', 'A1', 100), spot('s2', 'A2', 160)], decorations: [] };
const ZONE = {
  id: 'z1',
  map_id: 'm1',
  code: 'row1',
  name: { it: 'Prima fila' },
  assignment_mode: 'seat',
  online_visible: true,
  price_category: 'row1',
  auto_assign: false,
  sort_order: 0,
  spot_count: 2,
  version: 1,
  ...ts,
};
const COMPONENT = {
  id: 'c1',
  code: 'umbrella',
  name: { it: 'Ombrellone' },
  tracked: false,
  version: 1,
  ...ts,
};

interface SaveBody {
  spots: { id?: string; label: string; x: number; y: number }[];
}

function routes(role: 'manager' | 'cashier', extra: Record<string, Handler> = {}) {
  return {
    ...baseRoutes(role),
    [`GET ${API}/maps/m1`]: () => json(200, DOC, { ETag: '"3"' }),
    [`GET ${API}/zones`]: () => list([ZONE]),
    [`GET ${API}/components`]: () => list([COMPONENT]),
    [`GET ${API}/maps`]: () => list([MAP]),
    ...extra,
  };
}

/** PUT handler echoing the saved spots as the new server document. */
function echoPut(onBody?: (b: SaveBody, req: Request) => void): Handler {
  return async (req) => {
    const body = (await req.json()) as SaveBody;
    onBody?.(body, req);
    return json(200, {
      ...DOC,
      map: { ...MAP, version: 4 },
      spots: body.spots.map((s, i) => ({ ...spot(s.id ?? `new${i}`, s.label, s.x, s.y), ...s })),
    });
  };
}

const spotButton = (label: string) =>
  screen.findByRole('button', { name: new RegExp(`^Postazione ${label},`) });

describe('MapEditorPage', () => {
  it('moves a spot with the keyboard and saves the whole document with If-Match', async () => {
    const user = userEvent.setup();
    let sent: SaveBody | undefined;
    const { fetchMock } = renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', { [`PUT ${API}/maps/m1`]: echoPut((b) => (sent = b)) }),
    );
    const a1 = await spotButton('A1');
    expect(screen.getByText('2 postazioni')).toBeInTheDocument();
    a1.focus();
    await user.keyboard('{Enter}{ArrowRight}');
    expect(a1).toHaveAttribute('data-x', '110');
    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));
    await waitFor(() => expect(callsTo(fetchMock, 'PUT', `${API}/maps/m1`)).toHaveLength(1));
    const req = callsTo(fetchMock, 'PUT', `${API}/maps/m1`)[0] as Request;
    expect(req.headers.get('If-Match')).toBe('"3"');
    expect(sent?.spots.map((s) => [s.id, s.x])).toEqual([
      ['s1', 110],
      ['s2', 160],
    ]);
    expect(await screen.findByText('2 postazioni · salvata')).toBeInTheDocument();
  });

  it('multi-selects with shift, aligns, and undoes/redoes with the keyboard', async () => {
    const user = userEvent.setup();
    const d = { ...DOC, spots: [spot('s1', 'A1', 100, 200), spot('s2', 'A2', 160, 230)] };
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', { [`GET ${API}/maps/m1`]: () => json(200, d) }),
    );
    const a1 = await spotButton('A1');
    const a2 = await spotButton('A2');
    fireEvent.pointerDown(a1, { button: 0 });
    fireEvent.pointerDown(a2, { button: 0, shiftKey: true });
    expect(a1).toHaveAttribute('aria-pressed', 'true');
    expect(a2).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText('2 elementi selezionati')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Allinea in alto' }));
    expect(a2).toHaveAttribute('data-y', '200');

    await user.click(document.body);
    await user.keyboard('{Control>}z{/Control}');
    expect(a2).toHaveAttribute('data-y', '230');
    await user.keyboard('{Shift>}{Meta>}z{/Meta}{/Shift}');
    expect(a2).toHaveAttribute('data-y', '200');

    // Shift-click toggles an element out of the selection.
    fireEvent.pointerDown(a1, { button: 0, shiftKey: true });
    expect(a1).toHaveAttribute('aria-pressed', 'false');
    await user.keyboard('{Escape}');
    expect(a2).toHaveAttribute('aria-pressed', 'false');
  });

  it('adds from the palette, duplicates, edits properties and deletes', async () => {
    const user = userEvent.setup();
    let sent: SaveBody | undefined;
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', { [`PUT ${API}/maps/m1`]: echoPut((b) => (sent = b)) }),
    );
    await spotButton('A1');
    await user.click(screen.getByRole('button', { name: 'Aggiungi postazione: Gazebo' }));
    expect(screen.getByText(/^3 postazioni/)).toBeInTheDocument();
    expect(screen.getByLabelText('Etichetta')).toHaveValue('N3');

    const label = screen.getByLabelText('Etichetta');
    await user.clear(label);
    await user.type(label, 'G1{Enter}');
    expect(await spotButton('G1')).toBeInTheDocument();
    await user.type(screen.getByLabelText('Capienza: Ombrellone'), '2{Enter}');

    await user.click(document.body);
    await user.keyboard('{Control>}d{/Control}');
    expect(screen.getByText(/^4 postazioni/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Elimina' }));
    expect(screen.getByText(/^3 postazioni/)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));
    await waitFor(() => expect(sent).toBeDefined());
    const g1 = sent?.spots.find((s) => s.label === 'G1') as Record<string, unknown>;
    expect(g1).toMatchObject({ kind: 'gazebo', capacity: { umbrella: 2 } });
    expect(g1).not.toHaveProperty('id');
  });

  it('renumbers a selected row', async () => {
    const user = userEvent.setup();
    renderApp(`/admin/e/${EST_ID}/maps/m1`, routes('manager'));
    const a1 = await spotButton('A1');
    const a2 = await spotButton('A2');
    fireEvent.pointerDown(a2, { button: 0 });
    fireEvent.pointerDown(a1, { button: 0, shiftKey: true });
    await user.click(screen.getByRole('button', { name: 'Rinumera fila' }));
    const dialog = await screen.findByRole('dialog');
    const row = within(dialog).getByLabelText('Fila');
    await user.clear(row);
    await user.type(row, 'C');
    const start = within(dialog).getByLabelText('Primo numero');
    await user.clear(start);
    await user.type(start, '5');
    expect(within(dialog).getByText('2 postazioni: C5 … C6')).toBeInTheDocument();
    await user.click(within(dialog).getByRole('button', { name: 'Rinumera' }));
    expect(await spotButton('C5')).toHaveAttribute('data-x', '100');
    expect(await spotButton('C6')).toHaveAttribute('data-x', '160');
  });

  it('previews generated rows on the server and adds them to the draft', async () => {
    const user = userEvent.setup();
    let previewBody: Record<string, unknown> | undefined;
    const generated = ['B1', 'B2', 'B3'].map((label, i) => {
      const s = spot('x', label, 100 + i * 60, 140) as Record<string, unknown>;
      delete s.id;
      delete s.map_id;
      return s;
    });
    let sent: SaveBody | undefined;
    const { fetchMock } = renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', {
        [`POST ${API}/maps/m1/generate-rows`]: async (req) => {
          previewBody = (await req.json()) as Record<string, unknown>;
          return json(200, { spots: generated, conflicting_labels: [], applied: false });
        },
        [`PUT ${API}/maps/m1`]: echoPut((b) => (sent = b)),
      }),
    );
    await spotButton('A1');
    await user.click(screen.getByRole('button', { name: 'Generatore di file' }));
    const dialog = await screen.findByRole('dialog');
    const rows = within(dialog).getByLabelText('Postazioni per fila');
    await user.clear(rows);
    await user.type(rows, '3');
    await user.click(within(dialog).getByRole('button', { name: 'Anteprima' }));
    expect(await within(dialog).findByText('3 postazioni, da B1 a B3.')).toBeInTheDocument();
    expect(previewBody).toMatchObject({ zone_id: 'z1', rows: 1, spots_per_row: 3, apply: false });
    await user.click(within(dialog).getByRole('button', { name: 'Aggiungi 3 postazioni' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByText(/^5 postazioni/)).toBeInTheDocument();
    expect(await spotButton('B2')).toHaveAttribute('aria-pressed', 'true');

    // Applied locally: nothing written until the explicit save.
    expect(callsTo(fetchMock, 'PUT', `${API}/maps/m1`)).toHaveLength(0);
    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));
    await waitFor(() => expect(sent?.spots).toHaveLength(5));
    expect(sent?.spots.filter((s) => !s.id).map((s) => s.label)).toEqual(['B1', 'B2', 'B3']);
  });

  it('blocks generated labels that clash with the map', async () => {
    const user = userEvent.setup();
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', {
        [`POST ${API}/maps/m1/generate-rows`]: () =>
          json(200, {
            spots: [{ ...spot('x', 'A1', 10), id: undefined }],
            conflicting_labels: ['A1'],
            applied: false,
          }),
      }),
    );
    await spotButton('A1');
    await user.click(screen.getByRole('button', { name: 'Generatore di file' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Anteprima' }));
    expect(await within(dialog).findByText(/Etichette già presenti sulla mappa: A1/)).toBeVisible();
    expect(within(dialog).getByRole('button', { name: 'Aggiungi 1 postazione' })).toBeDisabled();
  });

  it('shows a diff on a version conflict and overwrites on the latest version', async () => {
    const user = userEvent.setup();
    let serverVersion = 3;
    const server = () => ({
      ...DOC,
      map: { ...MAP, version: serverVersion },
      spots:
        serverVersion === 3
          ? DOC.spots
          : [spot('s1', 'A1', 100), spot('s2', 'A2', 300), spot('s9', 'A9', 50)],
    });
    const puts: { ifMatch: string | null; body: SaveBody }[] = [];
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', {
        [`GET ${API}/maps/m1`]: () => json(200, server()),
        [`PUT ${API}/maps/m1`]: async (req) => {
          const body = (await req.json()) as SaveBody;
          const ifMatch = req.headers.get('If-Match');
          puts.push({ ifMatch, body });
          if (ifMatch !== `"${serverVersion}"`) {
            return problem(412, 'version_conflict', { current_version: serverVersion });
          }
          return json(200, {
            ...DOC,
            map: { ...MAP, version: serverVersion + 1 },
            spots: body.spots.map((s) => ({ ...spot(s.id ?? 'n', s.label, s.x, s.y), ...s })),
          });
        },
      }),
    );
    const a1 = await spotButton('A1');
    // Someone else saves version 4 meanwhile: A2 moved, A9 added.
    serverVersion = 4;
    fireEvent.keyDown(a1, { key: 'ArrowUp' });
    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));

    const dialog = await screen.findByRole('alertdialog', {
      name: 'Mappa modificata da un altro utente',
    });
    const yours = within(dialog).getByRole('region', { name: 'Le tue modifiche' });
    expect(within(yours).getByText('1 spostata:')).toBeInTheDocument();
    expect(within(yours).getByText('A1')).toBeInTheDocument();
    const theirs = await within(dialog).findByRole('region', {
      name: 'Salvate nel frattempo (versione 4)',
    });
    expect(within(theirs).getByText('1 aggiunta:')).toBeInTheDocument();
    expect(within(theirs).getByText('A9')).toBeInTheDocument();
    expect(within(theirs).getByText('1 spostata:')).toBeInTheDocument();

    await user.click(within(dialog).getByRole('button', { name: 'Sovrascrivi con la mia' }));
    await waitFor(() => expect(puts).toHaveLength(2));
    expect(puts[0]?.ifMatch).toBe('"3"');
    expect(puts[1]?.ifMatch).toBe('"4"');
    expect(puts[1]?.body.spots.map((s) => [s.id, s.y])).toEqual([
      ['s1', 190],
      ['s2', 200],
    ]);
    expect(await screen.findByText('2 postazioni · salvata')).toBeInTheDocument();
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('reloads the server version from the conflict dialog', async () => {
    const user = userEvent.setup();
    let version = 3;
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', {
        [`GET ${API}/maps/m1`]: () =>
          json(200, {
            ...DOC,
            map: { ...MAP, version },
            spots: version === 3 ? DOC.spots : [spot('s1', 'A1', 300)],
          }),
        [`PUT ${API}/maps/m1`]: () => problem(412, 'version_conflict', { current_version: 4 }),
      }),
    );
    const a2 = await spotButton('A2');
    version = 4;
    fireEvent.keyDown(a2, { key: 'ArrowUp' });
    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));
    const dialog = await screen.findByRole('alertdialog');
    await within(dialog).findByText('1 eliminata:');
    await user.click(within(dialog).getByRole('button', { name: 'Ricarica la versione salvata' }));
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /^Postazione A2,/ })).toBeNull(),
    );
    expect(await spotButton('A1')).toHaveAttribute('data-x', '300');
    expect(screen.getByText('1 postazione')).toBeInTheDocument();
  });

  it('lists the spots the server refuses to delete because of future allocations', async () => {
    const user = userEvent.setup();
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes('manager', {
        [`PUT ${API}/maps/m1`]: () =>
          problem(409, 'spot_has_future_allocations', { spot_ids: ['s2'] }),
      }),
    );
    const a2 = await spotButton('A2');
    fireEvent.keyDown(a2, { key: 'Delete' });
    expect(screen.queryByRole('button', { name: /^Postazione A2,/ })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));
    const alert = await screen.findByText(/Non puoi eliminare 1 postazione/);
    const box = alert.closest('[role="alert"]') as HTMLElement;
    expect(within(box).getByText('A2')).toBeInTheDocument();
  });

  it('is read-only for a cashier', async () => {
    renderApp(`/admin/e/${EST_ID}/maps/m1`, routes('cashier'));
    const a1 = await spotButton('A1');
    fireEvent.keyDown(a1, { key: 'ArrowRight' });
    expect(a1).toHaveAttribute('data-x', '100');
    expect(
      screen.queryByRole('button', { name: 'Aggiungi postazione: Ombrellone' }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Salva mappa' })).toBeDisabled();
  });
});
