/** Icon families for component codes (Italian and English codes are both common). */
const ICON_ALIASES: [RegExp, string][] = [
  [/umbrella|ombrellon|parasol|sonnenschirm/, 'umbrella'],
  [/sunbed|lettin|lounger|liege|transat/, 'sunbed'],
  [/deck|sdrai|regista/, 'deckchair'],
  [/chair|sedi|stuhl|chaise/, 'chair'],
  [/cabin|cabina|kabine/, 'cabin'],
  [/gazebo|pagod/, 'gazebo'],
  [/tent|tenda|zelt/, 'tent'],
  [/table|tavol|tisch/, 'table'],
];

/** Icon family for a component code, "generic" when unknown. */
export function iconKindFor(code: string): string {
  const c = code.toLowerCase();
  for (const [re, kind] of ICON_ALIASES) if (re.test(c)) return kind;
  return 'generic';
}
