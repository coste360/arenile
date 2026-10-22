import { describe, expect, it } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import { describeAction, describeConditions, describeRule, type SummaryNames } from './format';

const i18n = createAdminTestI18n();
const t = i18n.t.bind(i18n);
const names: SummaryNames = {
  zone: (id) => ({ z1: 'Prima fila', z2: 'Seconda fila' })[id] ?? id,
  agreement: (id) => ({ a1: 'Hotel Miramare' })[id] ?? id,
  target: (id) => ({ p1: 'Ombrellone + 2 lettini', p2: 'Gazebo' })[id] ?? id,
};
const nbsp = (s: string) => s.replace(/\u00a0/g, ' ');

describe('describeAction (it)', () => {
  it.each([
    [{ type: 'set' as const, amount: 2500 }, '25,00 € al giorno'],
    [{ type: 'add' as const, amount: 500 }, '+5,00 € al giorno'],
    [{ type: 'add' as const, amount: -250 }, '−2,50 € al giorno'],
    [{ type: 'multiply' as const, factor: '1.2' }, '× 1,2'],
    [{ type: 'percent_off' as const, percent: '12.5' }, '−12,5%'],
    [
      { type: 'amount_off' as const, amount: 1000, per: 'booking' as const },
      '−10,00 € sulla prenotazione',
    ],
    [{ type: 'amount_off' as const, amount: 100, per: 'day' as const }, '−1,00 € al giorno'],
    [{ type: 'free_days' as const, every: 7, free: 1 }, '7 giorni al prezzo di 6'],
  ])('%j → %s', (action, want) => {
    expect(nbsp(describeAction(t, action, 'it'))).toBe(want);
  });
});

describe('describeConditions (it)', () => {
  it('says "sempre" without conditions', () => {
    expect(describeConditions(t, {}, names)).toBe('sempre');
  });

  it('lists every condition in a readable form', () => {
    const text = describeConditions(
      t,
      {
        rate_periods: ['high', 'peak'],
        date_ranges: [{ from: '2027-08-10', to: '2027-08-20' }],
        weekdays: [6, 7],
        holidays: false,
        slots: ['AM'],
        formulas: ['day', 'multi_day'],
        price_categories: ['row1'],
        zone_ids: ['z1', 'z2'],
        spot_attributes: ['accessible'],
        channels: ['online'],
        min_days: 3,
        max_days: 1,
        lead_time_days: { min: 0, max: 1 },
        occupancy_percent: { min: 80 },
        customer_tags: ['storico'],
        agreement_ids: ['a1'],
      },
      names,
      'it',
    );
    expect(text.split(' · ')).toEqual([
      'periodi: high, peak',
      'dal 10 ago 2027 al 20 ago 2027',
      'giorni: sab, dom',
      'non festivi',
      'fascia: mattina',
      'formula: giornata, più giorni',
      'categorie: row1',
      'zone: Prima fila, Seconda fila',
      'postazioni con accessible',
      'canale: Online',
      'almeno 3 giorni',
      'al massimo 1 giorno',
      'prenotando da 0 a 1 giorni prima',
      'occupazione almeno 80%',
      'clienti storico',
      'convenzione: Hotel Miramare',
    ]);
  });
});

describe('describeRule (it)', () => {
  it('describes a base price for every pack', () => {
    expect(
      nbsp(
        describeRule(
          t,
          {
            stage: 'base',
            target_type: 'pack',
            conditions: { rate_periods: ['peak'], price_categories: ['row1'] },
            action: { type: 'set', amount: 3000 },
            stackable: false,
            active: true,
          },
          names,
          'it',
        ),
      ),
    ).toBe('Base per tutti i pack: 30,00 € al giorno, quando periodi: peak e categorie: row1.');
  });

  it('describes a stackable weekend modifier on a named pack', () => {
    expect(
      nbsp(
        describeRule(
          t,
          {
            stage: 'modifier',
            target_type: 'pack',
            target_ids: ['p1'],
            conditions: { weekdays: [6, 7], channels: ['online'], holidays: true },
            action: { type: 'add', amount: 500 },
            stackable: true,
            active: true,
          },
          names,
          'it',
        ),
      ),
    ).toBe(
      'Modificatore per Ombrellone + 2 lettini: +5,00 € al giorno, quando giorni: sab, dom, festivi e canale: Online. Cumulabile.',
    );
  });

  it('describes an inactive "7x6" discount that always applies', () => {
    expect(
      describeRule(
        t,
        {
          stage: 'discount',
          target_type: 'extra',
          conditions: {},
          action: { type: 'free_days', every: 7, free: 1 },
          stackable: false,
          active: false,
        },
        names,
        'it',
      ),
    ).toBe(
      'Sconto per tutti gli extra: 7 giorni al prezzo di 6, sempre. Non cumulabile. (disattivata)',
    );
  });
});
