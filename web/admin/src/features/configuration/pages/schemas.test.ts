import { describe, expect, it } from 'vitest';
import type { Establishment } from '@arenile/shared/api';
import { establishment } from '../testing';
import {
  componentSchema,
  extraSchema,
  packSchema,
  toExtraInput,
  toPackInput,
  toPackValues,
} from './catalog.schema';
import {
  closureBatchSchema,
  closureSchema,
  toClosureInput,
  toClosureInputs,
} from './closures.schema';
import {
  generalPatch,
  generalSchema,
  pricingSettingsPatch,
  pricingSettingsSchema,
  publicPagePatch,
  publicPageSchema,
  themePatch,
  themeSchema,
  toGeneralValues,
  toThemeValues,
} from './establishment.schema';
import { poolSchema, toPoolInput, toZoneInput, zoneSchema } from './inventory.schema';
import { newEstablishmentSchema, toEstablishmentCreate } from './newEstablishment.schema';
import {
  holidaysPatch,
  ratePeriodSchema,
  seasonSchema,
  timelineSegments,
  toRatePeriodInput,
  uncoveredDays,
} from './seasons.schema';
import { inviteSchema, toTeamMemberCreate } from './team.schema';

const est = establishment() as unknown as Establishment;

function messages(result: {
  success: boolean;
  error?: { issues: { path: PropertyKey[]; message: string }[] };
}) {
  return Object.fromEntries((result.error?.issues ?? []).map((i) => [i.path.join('.'), i.message]));
}

describe('establishment schemas', () => {
  it('general: round-trips the establishment and requires the default locale enabled', () => {
    const values = toGeneralValues(est);
    expect(generalSchema.safeParse(values).success).toBe(true);
    const bad = generalSchema.safeParse({ ...values, enabled_locales: ['en'], slug: 'Lido X' });
    expect(messages(bad)).toMatchObject({
      enabled_locales: 'configuration:validation.localeRequired',
      slug: 'configuration:validation.slug',
    });
    expect(generalPatch({ ...values, country: 'it' }).address?.country).toBe('IT');
  });

  it('public page: validates origins and deduplicates them', () => {
    expect(
      publicPageSchema.safeParse({ booking_enabled: true, allowed_embed_origins: 'https://a.it/x' })
        .success,
    ).toBe(false);
    expect(
      publicPagePatch({
        booking_enabled: true,
        allowed_embed_origins: 'https://a.it\nhttps://a.it/\nhttp://localhost:3000',
      }),
    ).toEqual({
      booking_enabled: true,
      allowed_embed_origins: ['https://a.it', 'http://localhost:3000'],
    });
  });

  it('theme: hex colours, https logo, keeps other custom texts', () => {
    const values = toThemeValues(
      establishment({
        theme: { custom_texts: { it: { terms: 'T' } } },
      }) as unknown as Establishment,
    );
    expect(values.primary_color).toBe('#0e6ba8');
    expect(themeSchema.safeParse({ ...values, primary_color: 'blue' }).success).toBe(false);
    expect(themeSchema.safeParse({ ...values, logo_url: 'http://x.it/l.png' }).success).toBe(false);
    const patch = themePatch(
      { ...values, intro: { it: 'Benvenuti', en: '' } },
      establishment({
        theme: { custom_texts: { it: { terms: 'T' } } },
      }) as unknown as Establishment,
    );
    expect(patch.theme?.custom_texts).toEqual({ it: { terms: 'T', intro: 'Benvenuti' } });
  });

  it('pricing settings: decimal factor, percent VAT to basis points, whole document kept', () => {
    expect(
      pricingSettingsSchema.safeParse({
        half_day_factor: '0',
        rounding: 'none',
        default_vat_rate: '22',
      }).success,
    ).toBe(false);
    expect(
      pricingSettingsSchema.safeParse({
        half_day_factor: '0,55',
        rounding: 'euro',
        default_vat_rate: '101',
      }).success,
    ).toBe(false);
    const patch = pricingSettingsPatch(
      { half_day_factor: '0,55', rounding: 'euro', default_vat_rate: '5,5' },
      establishment({
        settings: {
          pricing: {
            half_day_factor: '0.6',
            rounding: 'none',
            default_vat_rate_bp: 0,
            local_holidays: ['2027-08-16'],
          },
          default_policy: { max_spots_per_booking: 3 },
        },
      }) as unknown as Establishment,
    );
    expect(patch.settings).toEqual({
      pricing: {
        half_day_factor: '0.55',
        rounding: 'euro',
        default_vat_rate_bp: 550,
        local_holidays: ['2027-08-16'],
      },
      default_policy: { max_spots_per_booking: 3 },
    });
  });

  it('new establishment', () => {
    const v = {
      organization_id: '',
      name: 'Lido Due',
      slug: 'lido-due',
      timezone: 'Europe/Rome',
      default_locale: 'it' as const,
      enabled_locales: ['it' as const],
      contact_email: 'x',
      contact_phone: '',
    };
    expect(messages(newEstablishmentSchema.safeParse(v))).toEqual({
      contact_email: 'configuration:validation.email',
    });
    expect(toEstablishmentCreate({ ...v, contact_email: '' })).toEqual({
      name: 'Lido Due',
      slug: 'lido-due',
      timezone: 'Europe/Rome',
      currency: 'EUR',
      default_locale: 'it',
      enabled_locales: ['it'],
    });
  });
});

describe('seasons schemas', () => {
  const season = { opens_on: '2027-05-01', closes_on: '2027-09-30' };

  it('season dates in order', () => {
    expect(
      messages(
        seasonSchema.safeParse({
          year: '2027',
          opens_on: '2027-09-30',
          closes_on: '2027-05-01',
          status: 'draft',
        }),
      ),
    ).toEqual({ closes_on: 'configuration:validation.dateOrder' });
  });

  it('rate period: overlap with other periods, with itself, outside the season', () => {
    const schema = ratePeriodSchema({
      defaultLocale: 'it',
      season,
      others: [{ code: 'high', ranges: [{ from: '2027-07-01', to: '2027-08-31' }] }],
    });
    const base = { code: 'low', name: { it: 'Bassa' }, color: '#7dd3fc', sort_order: '0' };
    expect(
      schema.safeParse({ ...base, ranges: [{ from: '2027-05-01', to: '2027-06-30' }] }).success,
    ).toBe(true);
    expect(
      messages(schema.safeParse({ ...base, ranges: [{ from: '2027-06-15', to: '2027-07-05' }] })),
    ).toEqual({ 'ranges.0.from': 'configuration:validation.overlap' });
    expect(
      messages(
        schema.safeParse({
          ...base,
          ranges: [
            { from: '2027-05-01', to: '2027-05-20' },
            { from: '2027-05-10', to: '2027-05-30' },
          ],
        }),
      ),
    ).toMatchObject({ 'ranges.0.from': 'configuration:validation.overlap' });
    expect(
      messages(schema.safeParse({ ...base, ranges: [{ from: '2027-04-01', to: '2027-05-10' }] })),
    ).toEqual({ 'ranges.0.from': 'configuration:validation.outsideSeason' });
    expect(
      messages(
        schema.safeParse({
          ...base,
          name: { it: '' },
          ranges: [{ from: '2027-05-10', to: '2027-05-01' }],
        }),
      ),
    ).toEqual({
      'name.it': 'configuration:validation.defaultLocaleText',
      'ranges.0.to': 'configuration:validation.dateOrder',
    });
  });

  it('rate period input compacts names and parses the order', () => {
    expect(
      toRatePeriodInput(
        {
          code: 'peak',
          name: { it: 'Picco', en: '' },
          color: '#EF4444',
          sort_order: '',
          ranges: [{ from: '2027-08-08', to: '2027-08-22' }],
        },
        'season-1',
      ),
    ).toEqual({
      season_id: 'season-1',
      code: 'peak',
      name: { it: 'Picco' },
      color: '#ef4444',
      sort_order: undefined,
      ranges: [{ from: '2027-08-08', to: '2027-08-22' }],
    });
  });

  it('timeline segments and uncovered days', () => {
    const periods = [
      {
        id: 'a',
        code: 'low',
        color: '#111111',
        ranges: [{ from: '2027-05-01', to: '2027-06-30' }],
      },
      {
        id: 'b',
        code: 'high',
        color: '#222222',
        ranges: [{ from: '2027-07-01', to: '2027-08-31' }],
      },
    ];
    const seg = timelineSegments(season, periods);
    expect(seg.map((s) => s.code)).toEqual(['low', 'high']);
    expect(seg[0]?.left).toBe(0);
    expect(Math.round((seg[0]?.width ?? 0) * 153) / 100).toBeCloseTo(61, 0);
    expect(uncoveredDays(season, periods)).toBe(30);
  });

  it('local holidays patch keeps the rest of the settings and omits empty lists', () => {
    const patch = holidaysPatch(
      {
        local_holidays: [{ value: '2027-08-16' }, { value: '2027-08-16' }],
        recurring_local_holidays: [],
      },
      est,
    );
    expect(patch.settings?.pricing).toEqual({
      half_day_factor: '0.6',
      rounding: 'none',
      default_vat_rate_bp: 2200,
      local_holidays: ['2027-08-16'],
      recurring_local_holidays: undefined,
    });
  });
});

describe('closures', () => {
  it('requires a slot and orders them', () => {
    expect(closureSchema.safeParse({ date: '2027-08-10', slots: [], reason: '' }).success).toBe(
      false,
    );
    expect(toClosureInput({ date: '2027-08-10', slots: ['PM', 'AM'], reason: '' })).toEqual({
      date: '2027-08-10',
      slots: ['AM', 'PM'],
    });
  });

  it('creates one closure per selected day, sorted and deduplicated', () => {
    expect(closureBatchSchema.safeParse({ dates: [], slots: ['AM'], reason: '' }).success).toBe(
      false,
    );
    expect(
      toClosureInputs({
        dates: ['2027-08-11', '2027-08-10', '2027-08-11'],
        slots: ['PM'],
        reason: 'Allerta meteo',
      }),
    ).toEqual([
      { date: '2027-08-10', slots: ['PM'], reason: 'Allerta meteo' },
      { date: '2027-08-11', slots: ['PM'], reason: 'Allerta meteo' },
    ]);
  });
});

describe('zones and pools', () => {
  const zone = zoneSchema('it');
  const base = {
    map_id: 'm1',
    code: 'row1',
    name: { it: 'Prima fila' },
    assignment_mode: 'seat' as const,
    online_visible: true,
    quota_type: 'percent' as const,
    quota_value: '120',
    price_category: 'row1',
    auto_assign: true,
    sort_order: '',
  };

  it('validates the online quota', () => {
    expect(messages(zone.safeParse(base))).toEqual({
      quota_value: 'configuration:validation.percent',
    });
    expect(zone.safeParse({ ...base, quota_value: '70' }).success).toBe(true);
  });

  it('maps quantity zones without a map and drops auto-assign outside zone mode', () => {
    expect(toZoneInput({ ...base, quota_value: '70' })).toMatchObject({
      map_id: 'm1',
      online_quota: { type: 'percent', value: 70 },
      auto_assign: false,
    });
    const q = toZoneInput({ ...base, assignment_mode: 'quantity', quota_type: 'none' });
    expect(q.map_id).toBeUndefined();
    expect(q.online_quota).toBeUndefined();
  });

  it('pool overrides: dates in order, nil when empty', () => {
    const pool = poolSchema('it');
    const v = {
      zone_id: '',
      component_code: 'sunbed',
      name: { it: 'Lettini' },
      capacity: '40',
      capacity_overrides: [],
    };
    expect(pool.safeParse(v).success).toBe(true);
    expect(
      pool.safeParse({
        ...v,
        capacity_overrides: [{ from: '2027-08-31', to: '2027-08-01', capacity: '50' }],
      }).success,
    ).toBe(false);
    expect(toPoolInput(v)).toEqual({
      component_code: 'sunbed',
      name: { it: 'Lettini' },
      capacity: 40,
    });
  });
});

describe('catalog', () => {
  it('component codes', () => {
    expect(
      componentSchema('it').safeParse({
        code: 'Ombrellone Grande',
        name: { it: 'x' },
        tracked: false,
        pool_id: '',
      }).success,
    ).toBe(false);
  });

  it('pack composition: required, positive, unique; VAT in basis points; nil lists', () => {
    const schema = packSchema('it');
    const values = { ...toPackValues(undefined, ['it']), code: 'base', name: { it: 'Base' } };
    expect(messages(schema.safeParse(values))).toEqual({
      composition: 'configuration:validation.required',
    });
    const dup = {
      ...values,
      composition: [
        { component_code: 'umbrella', quantity: '1' },
        { component_code: 'umbrella', quantity: '0' },
      ],
    };
    expect(messages(schema.safeParse(dup))).toEqual({
      'composition.1.component_code': 'configuration:validation.duplicate',
      'composition.1.quantity': 'configuration:validation.integer',
    });
    const ok = {
      ...values,
      composition: [
        { component_code: 'umbrella', quantity: '1' },
        { component_code: 'sunbed', quantity: '2' },
      ],
      vat_rate: '10',
    };
    expect(schema.safeParse(ok).success).toBe(true);
    expect(toPackInput(ok)).toMatchObject({
      composition: { umbrella: 1, sunbed: 2 },
      vat_rate_bp: 1000,
      requires_spot_kind: undefined,
      allowed_zone_ids: undefined,
      channels: ['desk', 'online'],
    });
  });

  it('extra: allowed packs only for line extras, channels required', () => {
    const schema = extraSchema('it');
    const v = {
      code: 'safe',
      name: { it: 'Cassetta' },
      description: {},
      component_code: '',
      pricing_unit: 'per_day' as const,
      max_quantity: '',
      attach_to: 'booking' as const,
      allowed_pack_ids: ['p1'],
      channels: [] as ('desk' | 'online' | 'api')[],
      sort_order: '',
      active: true,
      vat_rate: '',
    };
    expect(messages(schema.safeParse(v))).toEqual({
      channels: 'configuration:validation.required',
    });
    expect(toExtraInput({ ...v, channels: ['desk'] })).toMatchObject({
      allowed_pack_ids: undefined,
      vat_rate_bp: undefined,
    });
  });
});

describe('team', () => {
  it('invite: email and optional password of 12+ characters', () => {
    const v = {
      email: 'cassa@lido.it',
      name: 'Anna',
      role: 'cashier' as const,
      scope: 'establishment' as const,
      locale: 'it' as const,
      password: 'short',
    };
    expect(messages(inviteSchema.safeParse(v))).toEqual({
      password: 'configuration:validation.passwordLength',
    });
    expect(toTeamMemberCreate({ ...v, password: '' })).toEqual({
      email: 'cassa@lido.it',
      name: 'Anna',
      role: 'cashier',
      scope: 'establishment',
      locale: 'it',
    });
  });
});
