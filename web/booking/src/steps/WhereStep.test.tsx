import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useReducer } from 'react';
import { describe, expect, it } from 'vitest';
import { flowReducer, initialFlowState, type FlowState } from '../flow/selection';
import {
  BASE,
  CARD_ZONE,
  MAP_DOC,
  MAP_ID,
  QTY_ZONE,
  SPOT_A1,
  availabilityFixture,
  establishmentFixture,
  json,
  renderWithEnv,
  type ZoneSet,
} from '../test-utils';
import WhereStep from './WhereStep';

function start(): FlowState {
  return flowReducer(initialFlowState({ from: '2099-07-10', to: '2099-07-12' }), {
    type: 'formula',
    formula: 'multi_day',
  });
}

function Harness({ initial = start() }: { initial?: FlowState }) {
  const [state, dispatch] = useReducer(flowReducer, initial);
  return (
    <>
      <WhereStep state={state} dispatch={dispatch} />
      <output data-testid="lines">
        {state.lines.map((l) => `${l.key}:${l.quantity}`).join(',')}
      </output>
    </>
  );
}

function renderWhere(modes: ZoneSet[], availability = availabilityFixture(), initial?: FlowState) {
  const establishment = establishmentFixture({}, modes);
  return renderWithEnv(
    <Harness initial={initial} />,
    {
      [`GET ${BASE}/availability`]: () => json(200, availability),
      [`GET ${BASE}/maps/${MAP_ID}`]: () => json(200, MAP_DOC),
    },
    { establishment, online: establishment.online! },
  );
}

const lines = () => screen.getByTestId('lines').textContent;

describe('WhereStep', () => {
  it('seat zones: map with the kit art, free spots selectable, price legend per row', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderWhere(['seat']);
    expect(await screen.findByTestId('beach-map')).toBeInTheDocument();
    const legend = screen.getByRole('list', { name: 'Legenda dei prezzi' });
    expect(within(legend).getByText('Fila A')).toBeInTheDocument();
    expect(legend).toHaveTextContent('da 35,00 €');
    expect(legend).toHaveTextContent('da 25,00 €');
    const taken = screen.getByRole('button', { name: /Postazione A2: Non disponibile/ });
    expect(taken).toHaveAttribute('aria-disabled', 'true');
    // Spots are 60 units (≥ 44 px in jsdom): a tap selects directly.
    await user.click(screen.getByRole('button', { name: /Postazione A1: Libero, da 35,00\s€/ }));
    expect(lines()).toBe(`${SPOT_A1}:1`);
    expect(screen.getByText(/^1 postazione scelta: \w+/)).toBeInTheDocument();
    expect(screen.queryByText('Zone')).not.toBeInTheDocument();
    const url = new URL((fetchMock.mock.calls[0]![0] as Request).url);
    expect(Object.fromEntries(url.searchParams)).toEqual({
      from: '2099-07-10',
      to: '2099-07-12',
      slots: 'FULL',
      formula: 'multi_day',
    });
  });

  it('offers a list alternative to the map', async () => {
    const user = userEvent.setup();
    renderWhere(['seat']);
    await user.click(await screen.findByRole('button', { name: 'Mostra come elenco' }));
    const list = screen.getByRole('region', { name: 'Elenco delle postazioni' });
    await user.click(within(list).getByRole('button', { name: /B1 – Libero – da 25,00\s€/ }));
    expect(lines()).toContain(':1');
    expect(within(list).getByRole('button', { name: /A2/ })).toBeDisabled();
  });

  it('zone zones: cards with "da" price and "ultimi N posti", no map', async () => {
    const user = userEvent.setup();
    renderWhere(['zone']);
    const card = await screen.findByRole('button', { name: /Pineta/ });
    expect(card).toHaveTextContent('da 25,00 €');
    expect(card).toHaveTextContent('Ultimi 3 posti');
    expect(screen.queryByTestId('beach-map')).not.toBeInTheDocument();
    expect(screen.getByText(/assegnata automaticamente/)).toBeInTheDocument();
    await user.click(card);
    expect(card).toHaveAttribute('aria-pressed', 'true');
    expect(lines()).toBe(`${CARD_ZONE}:1`);
  });

  it('disables sold-out zone cards', async () => {
    renderWhere(['zone'], availabilityFixture({ cardIndicator: 'none' }));
    expect(await screen.findByRole('button', { name: /Pineta/ })).toBeDisabled();
  });

  it('quantity zones: stepper limited to the available units', async () => {
    const user = userEvent.setup();
    renderWhere(['quantity']);
    const plus = await screen.findByRole('button', { name: 'Aumenta Lettini in riva' });
    expect(screen.getByText('4 disponibili')).toBeInTheDocument();
    for (let i = 0; i < 6; i++) await user.click(plus);
    expect(lines()).toBe(`${QTY_ZONE}:4`);
    expect(plus).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Diminuisci Lettini in riva' }));
    expect(lines()).toBe(`${QTY_ZONE}:3`);
  });

  it('mixed modes show the map, the cards and the steppers together', async () => {
    renderWhere(['seat', 'zone', 'quantity']);
    expect(await screen.findByTestId('beach-map')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Zone' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Quantità' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Spiaggia' })).toBeInTheDocument();
  });

  it('removes a chosen spot that became unavailable, with a notice', async () => {
    const initial = flowReducer(start(), {
      type: 'toggleSpot',
      zoneId: MAP_DOC.spots[0]!.zone_id,
      spotId: SPOT_A1,
      spotLabel: 'A1',
    });
    renderWhere(['seat'], availabilityFixture({ a1Free: false }), initial);
    expect(await screen.findByText(/non sono più disponibili/)).toBeInTheDocument();
    expect(lines()).toBe('');
  });
});
