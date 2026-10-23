import type { Schemas } from '@arenile/shared/api';
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useReducer } from 'react';
import { describe, expect, it } from 'vitest';
import { parseBookingLocation } from '../location';
import { flowReducer, initialFlowState, toSelection, type FlowState } from '../flow/selection';
import {
  BASE,
  CARD_ZONE,
  CATALOG,
  EXTRA_PARKING,
  EXTRA_SUNBED,
  PACK_BASE,
  PACK_VIP,
  json,
  renderWithEnv,
} from '../test-utils';
import WhatStep from './WhatStep';

function start(): FlowState {
  let s = initialFlowState({ from: '2099-07-10', to: '2099-07-12' });
  s = flowReducer(s, { type: 'formula', formula: 'multi_day' });
  return flowReducer(s, { type: 'toggleZone', zoneId: CARD_ZONE });
}

function Harness({ promo }: { promo?: Schemas['PromoStatus'] }) {
  const [state, dispatch] = useReducer(flowReducer, undefined, start);
  return (
    <>
      <WhatStep state={state} dispatch={dispatch} promoStatus={promo} />
      <output data-testid="selection">{JSON.stringify(toSelection(state))}</output>
    </>
  );
}

const selection = () => JSON.parse(screen.getByTestId('selection').textContent ?? 'null');

describe('WhatStep', () => {
  it('lists compatible packs with composition, then line and booking extras', async () => {
    const user = userEvent.setup();
    const { fetchMock } = renderWithEnv(<Harness />, {
      [`GET ${BASE}/catalog`]: () => json(200, CATALOG),
    });
    const group = await screen.findByRole('radiogroup', { name: /Pineta/ });
    const base = within(group).getByRole('radio', { name: /Base/ });
    expect(base).toHaveTextContent('2 × Lettino');
    expect(base).toHaveTextContent('da 25,00');
    expect(within(group).getByRole('radio', { name: /VIP/ })).toHaveTextContent(
      'Su conferma dello stabilimento',
    );
    expect(selection()).toBeNull();
    expect(screen.queryByText('Lettino aggiuntivo')).not.toBeInTheDocument();

    await user.click(base);
    expect(base).toHaveAttribute('aria-checked', 'true');
    await user.click(screen.getByRole('button', { name: 'Aumenta Lettino aggiuntivo' }));
    await user.click(screen.getByRole('button', { name: 'Aumenta Parcheggio' }));
    expect(selection()).toMatchObject({
      lines: [
        {
          pack_id: PACK_BASE,
          zone_id: CARD_ZONE,
          extras: [{ extra_id: EXTRA_SUNBED, quantity: 1 }],
        },
      ],
      extras: [{ extra_id: EXTRA_PARKING, quantity: 1 }],
    });
    // The line catalog is asked with the zone of the line.
    const zoneQuery = fetchMock.mock.calls
      .map((c) => new URL((c[0] as Request).url))
      .find((u) => u.searchParams.get('zone_id'));
    expect(zoneQuery?.searchParams.get('zone_id')).toBe(CARD_ZONE);
  });

  it('moves between packs with the arrow keys (radio group)', async () => {
    const user = userEvent.setup();
    renderWithEnv(<Harness />, { [`GET ${BASE}/catalog`]: () => json(200, CATALOG) });
    const base = await screen.findByRole('radio', { name: /Base/ });
    base.focus();
    await user.keyboard('{ArrowDown}');
    expect(screen.getByRole('radio', { name: /VIP/ })).toHaveAttribute('aria-checked', 'true');
    expect(document.activeElement).toBe(screen.getByRole('radio', { name: /VIP/ }));
  });

  it('preselects the pack of ?pack= or the only compatible one', async () => {
    renderWithEnv(
      <Harness />,
      { [`GET ${BASE}/catalog`]: () => json(200, CATALOG) },
      { location: parseBookingLocation('/b/lido-demo', '?pack=vip') },
    );
    expect(await screen.findByRole('radio', { name: /VIP/ })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    expect(selection().lines[0].pack_id).toBe(PACK_VIP);
  });

  it('applies a promo code and shows its status from the quote', async () => {
    const user = userEvent.setup();
    const { rerender } = renderWithEnv(<Harness />, {
      [`GET ${BASE}/catalog`]: () => json(200, { ...CATALOG, packs: [CATALOG.packs[0]] }),
    });
    expect(await screen.findByRole('radio', { name: /Base/ })).toHaveAttribute(
      'aria-checked',
      'true',
    );
    await user.type(screen.getByLabelText('Codice promozionale'), 'estate');
    await user.click(screen.getByRole('button', { name: 'Applica' }));
    expect(selection().promo_code).toBe('ESTATE');
    rerender(<Harness promo={{ code: 'ESTATE', status: 'invalid' }} />);
    expect(screen.getByText('Codice non valido per questa prenotazione.')).toBeInTheDocument();
  });
});
