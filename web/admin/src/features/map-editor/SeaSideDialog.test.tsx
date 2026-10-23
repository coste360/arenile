import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  EST_ID,
  baseRoutes,
  callsTo,
  json,
  list,
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
const spot = (id: string, label: string, x: number, y: number) => ({
  id,
  map_id: 'm1',
  zone_id: 'z1',
  label,
  row_label: label.slice(0, 1),
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
// Sea at the bottom: row A (y 260) is nearer to the sea than row B (y 200).
const DOC = {
  map: MAP,
  spots: [spot('s1', 'A1', 100, 260), spot('s2', 'B1', 100, 200)],
  decorations: [],
};
// The same layout mirrored by the server (sea at the top).
const FLIPPED = {
  map: { ...MAP, sea_side: 'top', version: 4 },
  spots: [spot('s1', 'A1', 100, 40), spot('s2', 'B1', 100, 100)],
  decorations: [],
};
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

interface ReorientBody {
  sea_side: string;
  dry_run?: boolean;
  acknowledge_future_allocations?: boolean;
}

/** Reorient handler: dry runs answer with the preview, the rest applies. */
function reorient(bodies: ReorientBody[], future = 0): Handler {
  return async (req) => {
    const body = (await req.json()) as ReorientBody;
    bodies.push(body);
    const applied = !body.dry_run;
    return json(
      200,
      {
        document: applied ? FLIPPED : { ...FLIPPED, map: { ...FLIPPED.map, version: 3 } },
        from_sea_side: 'bottom',
        to_sea_side: body.sea_side,
        transform: 'mirror_y',
        applied,
        future_allocations: future,
        spots_with_future_allocations: future ? 1 : 0,
      },
      { ETag: applied ? '"4"' : '"3"' },
    );
  };
}

function routes(extra: Record<string, Handler> = {}, doc: unknown = DOC) {
  return {
    ...baseRoutes('manager'),
    [`GET ${API}/maps/m1`]: () => json(200, doc, { ETag: '"3"' }),
    [`GET ${API}/zones`]: () => list([ZONE]),
    [`GET ${API}/components`]: () => list([]),
    [`GET ${API}/maps`]: () => list([MAP]),
    ...extra,
  };
}

const spotButton = (label: string) =>
  screen.findByRole('button', { name: new RegExp(`^Postazione ${label},`) });

/** Opens the map settings and moves the sea to the top. */
async function moveSeaToTop(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Impostazioni mappa' }));
  const settings = await screen.findByRole('dialog');
  await user.click(within(settings).getByRole('combobox', { name: 'Lato del mare' }));
  await user.click(await screen.findByRole('option', { name: 'In alto' }));
  await user.click(within(settings).getByRole('button', { name: 'Applica' }));
  return screen.findByRole('alertdialog', { name: 'Spostare il mare?' });
}

describe('Sea side change in the map editor', () => {
  it('flips the whole layout on the server after a dry run, acknowledging future bookings', async () => {
    const user = userEvent.setup();
    const bodies: ReorientBody[] = [];
    const { fetchMock } = renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes({ [`POST ${API}/maps/m1/reorient`]: reorient(bodies, 2) }),
    );
    await spotButton('A1');
    const dialog = await moveSeaToTop(user);
    expect(within(dialog).getByText(/da «In basso» a «In alto»/)).toBeInTheDocument();
    // The dry run counts the future allocations and nothing is saved yet.
    expect(
      await within(dialog).findByText(/2 prenotazioni, abbonamenti o blocchi/),
    ).toBeInTheDocument();
    expect(bodies).toEqual([
      { sea_side: 'top', dry_run: true, acknowledge_future_allocations: false },
    ]);

    await user.click(within(dialog).getByRole('button', { name: 'Ribalta la disposizione' }));
    await waitFor(() => expect(bodies).toHaveLength(2));
    expect(bodies[1]).toEqual({
      sea_side: 'top',
      dry_run: false,
      acknowledge_future_allocations: true,
    });
    const calls = callsTo(fetchMock, 'POST', `${API}/maps/m1/reorient`);
    expect((calls[1] as Request).headers.get('If-Match')).toBe('"3"');

    // The saved, flipped document replaces the draft: row A is now at the top.
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(await spotButton('A1')).toHaveAttribute('data-y', '40');
    expect(await screen.findByText('2 postazioni · salvata')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Salva mappa' })).toBeDisabled();
  });

  it('moves only the sea in the draft, saved with the map', async () => {
    const user = userEvent.setup();
    const bodies: ReorientBody[] = [];
    let saved: { map: { sea_side: string }; spots: { y: number }[] } | undefined;
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes({
        [`POST ${API}/maps/m1/reorient`]: reorient(bodies),
        [`PUT ${API}/maps/m1`]: async (req) => {
          saved = (await req.json()) as typeof saved;
          return json(200, { ...DOC, map: { ...MAP, sea_side: 'top', version: 4 } });
        },
      }),
    );
    await spotButton('A1');
    const dialog = await moveSeaToTop(user);
    await waitFor(() => expect(bodies).toHaveLength(1));
    expect(within(dialog).queryByText('Prenotazioni future sulla mappa')).toBeNull();
    await user.click(within(dialog).getByRole('button', { name: 'Solo il mare' }));
    expect(await screen.findByText(/modifiche non salvate/)).toBeInTheDocument();
    expect(await spotButton('A1')).toHaveAttribute('data-y', '260');
    await user.click(screen.getByRole('button', { name: 'Salva mappa' }));
    await waitFor(() => expect(saved?.map.sea_side).toBe('top'));
    expect(saved?.spots.map((s) => s.y)).toEqual([260, 200]);
    expect(bodies.every((b) => b.dry_run)).toBe(true);
  });

  it('does not flip a layout with unsaved changes', async () => {
    const user = userEvent.setup();
    const bodies: ReorientBody[] = [];
    renderApp(
      `/admin/e/${EST_ID}/maps/m1`,
      routes({ [`POST ${API}/maps/m1/reorient`]: reorient(bodies) }),
    );
    const a1 = await spotButton('A1');
    fireEvent.keyDown(a1, { key: 'ArrowRight' });
    const dialog = await moveSeaToTop(user);
    expect(
      within(dialog).getByText(/modifiche non salvate: salvale o annullale/),
    ).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Ribalta la disposizione' })).toBeDisabled();
    expect(bodies).toHaveLength(0);
    await user.click(within(dialog).getByRole('button', { name: 'Annulla' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
  });

  it('changes the sea of an empty map without asking', async () => {
    const user = userEvent.setup();
    renderApp(`/admin/e/${EST_ID}/maps/m1`, routes({}, { ...DOC, spots: [] }));
    await screen.findByText('0 postazioni');
    await user.click(screen.getByRole('button', { name: 'Impostazioni mappa' }));
    const settings = await screen.findByRole('dialog');
    await user.click(within(settings).getByRole('combobox', { name: 'Lato del mare' }));
    await user.click(await screen.findByRole('option', { name: 'In alto' }));
    await user.click(within(settings).getByRole('button', { name: 'Applica' }));
    expect(await screen.findByText(/modifiche non salvate/)).toBeInTheDocument();
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });
});
