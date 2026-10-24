import { z } from 'zod';
import type { Printer, PrinterCodepage, PrinterCutMode, PrinterInput } from './api';
import { checkHost, DEFAULT_PORT, defaultCharsPerLine, type PaperWidth } from './lib';

/** Codepages offered in the form; cp858 has € and the Italian accented vowels. */
export const CODEPAGES = [
  'cp858',
  'cp437',
  'cp850',
  'cp852',
  'cp1252',
] as const satisfies readonly PrinterCodepage[];
export const CUT_MODES = ['full', 'partial', 'none'] as const satisfies readonly PrinterCutMode[];

const M = {
  required: 'printers:validation.required',
  host: 'printers:validation.host',
  hostPublic: 'printers:validation.hostPublic',
  port: 'printers:validation.port',
  chars: 'printers:validation.chars',
  feed: 'printers:validation.feed',
  table: 'printers:validation.table',
} as const;

const intIn = (min: number, max: number, message: string) =>
  z
    .string()
    .trim()
    .refine((v) => /^\d+$/.test(v) && Number(v) >= min && Number(v) <= max, message);

/** Printer form (zod messages are i18n keys). Numbers are typed as text and parsed on submit. */
export const printerSchema = z.object({
  name: z.string().trim().min(1, M.required).max(80),
  host: z
    .string()
    .trim()
    .min(1, M.required)
    .max(253)
    .superRefine((v, ctx) => {
      const check = checkHost(v);
      if (check === 'invalid') ctx.addIssue({ code: 'custom', message: M.host });
      if (check === 'public') ctx.addIssue({ code: 'custom', message: M.hostPublic });
    }),
  port: intIn(1, 65535, M.port),
  paper_width_mm: z.enum(['58', '80']),
  chars_per_line: intIn(24, 64, M.chars),
  codepage: z.enum(CODEPAGES),
  codepage_table: z
    .string()
    .trim()
    .refine((v) => v === '' || (/^\d+$/.test(v) && Number(v) <= 255), M.table),
  cut_mode: z.enum(CUT_MODES),
  feed_lines: intIn(0, 10, M.feed),
  print_logo: z.boolean(),
  drawer_pin: z.enum(['none', '2', '5']),
  active: z.boolean(),
});

export type PrinterValues = z.infer<typeof printerSchema>;

export function toPrinterValues(p?: Printer): PrinterValues {
  const width = String(p?.paper_width_mm ?? 80) as PrinterValues['paper_width_mm'];
  return {
    name: p?.name ?? '',
    host: p?.host ?? '',
    port: String(p?.port ?? DEFAULT_PORT),
    paper_width_mm: width,
    chars_per_line: String(p?.chars_per_line ?? defaultCharsPerLine(Number(width) as PaperWidth)),
    codepage: p?.codepage ?? 'cp858',
    codepage_table: p?.codepage_table === undefined ? '' : String(p.codepage_table),
    cut_mode: p?.cut_mode ?? 'partial',
    feed_lines: String(p?.feed_lines ?? 4),
    print_logo: p?.print_logo ?? true,
    drawer_pin: p?.drawer_pin === undefined ? 'none' : (String(p.drawer_pin) as '2' | '5'),
    active: p?.active ?? true,
  };
}

export function toPrinterInput(v: PrinterValues): PrinterInput {
  return {
    name: v.name.trim(),
    host: v.host.trim(),
    port: Number(v.port),
    paper_width_mm: Number(v.paper_width_mm) as PaperWidth,
    chars_per_line: Number(v.chars_per_line),
    codepage: v.codepage,
    ...(v.codepage_table === '' ? {} : { codepage_table: Number(v.codepage_table) }),
    cut_mode: v.cut_mode,
    feed_lines: Number(v.feed_lines),
    print_logo: v.print_logo,
    ...(v.drawer_pin === 'none' ? {} : { drawer_pin: Number(v.drawer_pin) as 2 | 5 }),
    active: v.active,
  };
}
