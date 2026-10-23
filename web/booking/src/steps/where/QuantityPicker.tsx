export interface QuantityPickerProps {
  zoneId: string;
  /** Maximum units available online for the whole interval. */
  max: number;
  value: number;
  onChange: (value: number) => void;
}

/** Quantity selector of a `quantity` zone (spec 12.3). Owner: booking app builder. */
export function QuantityPicker(_props: QuantityPickerProps) {
  return null;
}
