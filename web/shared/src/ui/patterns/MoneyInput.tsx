import { useState, type ComponentProps } from 'react';
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  InputGroupText,
} from '@arenile/shared/ui/input-group';
import { cn } from '@arenile/shared/lib/utils';
import {
  basisPointsToPercentText,
  centsToEuroText,
  parseScaled,
  type ParseOptions,
} from '@arenile/shared/lib/money';

type NativeProps = Omit<
  ComponentProps<'input'>,
  'value' | 'defaultValue' | 'onChange' | 'type' | 'inputMode'
>;

export interface ScaledInputProps extends NativeProps {
  /** Integer value (cents or basis points); null when empty. */
  value: number | null | undefined;
  /** Called with the parsed integer, or null when empty or not a valid number. */
  onChange: (value: number | null) => void;
  allowNegative?: boolean;
}

interface Config {
  scale: number;
  addon: string;
  toText: (v: number) => string;
}

function useScaledText(
  value: number | null | undefined,
  onChange: (v: number | null) => void,
  { scale, toText }: Config,
  options: ParseOptions,
) {
  const [text, setText] = useState(() => (value == null ? '' : toText(value)));
  const [focused, setFocused] = useState(false);
  const parsed = parseScaled(text, scale, options);
  const invalid = text.trim() !== '' && parsed === null;

  // Follow external changes (form reset) while the user is not typing; changes
  // made by typing (including null for unparsable text) leave the text alone,
  // so an invalid entry stays visible after blur and can be corrected.
  const [prevValue, setPrevValue] = useState(value);
  if (value !== prevValue) {
    setPrevValue(value);
    if (!focused && (value ?? null) !== parsed) setText(value == null ? '' : toText(value));
  }

  return {
    text,
    invalid,
    onTextChange: (next: string) => {
      setText(next);
      onChange(parseScaled(next, scale, options));
    },
    onFocus: () => setFocused(true),
    onBlur: () => {
      setFocused(false);
      if (parsed !== null) setText(toText(parsed));
    },
  };
}

function ScaledInput({
  value,
  onChange,
  allowNegative,
  config,
  className,
  onFocus,
  onBlur,
  'aria-invalid': ariaInvalid,
  ...props
}: ScaledInputProps & { config: Config }) {
  const s = useScaledText(value, onChange, config, { allowNegative });
  return (
    <InputGroup className={cn('max-w-48', className)} data-disabled={props.disabled || undefined}>
      <InputGroupInput
        {...props}
        type="text"
        inputMode="decimal"
        autoComplete="off"
        className="text-right tabular-nums"
        value={s.text}
        aria-invalid={ariaInvalid ?? (s.invalid || undefined)}
        onChange={(e) => s.onTextChange(e.target.value)}
        onFocus={(e) => {
          s.onFocus();
          onFocus?.(e);
        }}
        onBlur={(e) => {
          s.onBlur();
          onBlur?.(e);
        }}
      />
      <InputGroupAddon align="inline-end">
        <InputGroupText aria-hidden="true">{config.addon}</InputGroupText>
      </InputGroupAddon>
    </InputGroup>
  );
}

const MONEY: Config = { scale: 2, addon: '€', toText: centsToEuroText };
const PERCENT: Config = { scale: 2, addon: '%', toText: basisPointsToPercentText };

/**
 * Euro amount bound to integer cents ("1.234,50" ⇄ 123450). Parsing is exact
 * (no floats); on blur the text is normalised to Italian formatting.
 * With react-hook-form: `<MoneyInput value={field.value} onChange={field.onChange} />`.
 */
export function MoneyInput(props: ScaledInputProps) {
  return <ScaledInput {...props} config={MONEY} />;
}

/** Percentage bound to integer basis points ("5,5" ⇄ 550, "22" ⇄ 2200). */
export function PercentInput(props: ScaledInputProps) {
  return <ScaledInput {...props} config={PERCENT} />;
}
