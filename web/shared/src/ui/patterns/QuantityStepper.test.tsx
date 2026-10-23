import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { renderWithI18n } from '../../testing';
import { ExtraList, ExtraRow } from './ExtraRow';
import { clampQuantity, QuantityStepper } from './QuantityStepper';

function Harness({
  initial = 0,
  min = 0,
  max = 2,
  onValue = () => {},
}: {
  initial?: number;
  min?: number;
  max?: number;
  onValue?: (n: number) => void;
}) {
  const [v, setV] = useState(initial);
  return (
    <QuantityStepper
      label="Lettino aggiuntivo"
      min={min}
      max={max}
      value={v}
      onChange={(n) => {
        setV(n);
        onValue(n);
      }}
    />
  );
}

describe('clampQuantity', () => {
  it.each([
    [3, 0, 2, 2],
    [-1, 0, 2, 0],
    [Number.NaN, 0, 2, 0],
    [1, 1, 0, 1],
  ])('clamp(%s, %s, %s) = %s', (n, min, max, out) => expect(clampQuantity(n, min, max)).toBe(out));
});

describe('QuantityStepper', () => {
  it('has accessible names and stops at the bounds', async () => {
    const onValue = vi.fn();
    renderWithI18n(<Harness onValue={onValue} />);
    const user = userEvent.setup();
    const value = screen.getByRole('spinbutton', { name: 'Lettino aggiuntivo' });
    const minus = screen.getByRole('button', { name: 'Diminuisci Lettino aggiuntivo' });
    const plus = screen.getByRole('button', { name: 'Aumenta Lettino aggiuntivo' });
    expect(value).toHaveTextContent('0');
    expect(value).toHaveAttribute('aria-valuemin', '0');
    expect(value).toHaveAttribute('aria-valuemax', '2');
    expect(minus).toBeDisabled();
    await user.click(plus);
    await user.click(plus);
    expect(value).toHaveAttribute('aria-valuenow', '2');
    expect(plus).toBeDisabled();
    expect(minus).toBeEnabled();
    expect(onValue.mock.calls).toEqual([[1], [2]]);
  });

  it('supports the spinbutton keys without leaving the range', async () => {
    const onValue = vi.fn();
    renderWithI18n(<Harness max={5} onValue={onValue} />);
    const user = userEvent.setup();
    const value = screen.getByRole('spinbutton', { name: 'Lettino aggiuntivo' });
    value.focus();
    await user.keyboard('{ArrowUp}{ArrowRight}');
    expect(value).toHaveAttribute('aria-valuenow', '2');
    await user.keyboard('{End}');
    expect(value).toHaveAttribute('aria-valuenow', '5');
    await user.keyboard('{ArrowUp}{PageUp}');
    expect(value).toHaveAttribute('aria-valuenow', '5');
    await user.keyboard('{ArrowDown}');
    expect(value).toHaveAttribute('aria-valuenow', '4');
    await user.keyboard('{Home}{ArrowLeft}');
    expect(value).toHaveAttribute('aria-valuenow', '0');
    expect(onValue.mock.calls.flat()).toEqual([1, 2, 5, 4, 0]);
  });

  it('always shows a value, clamping an out-of-range or missing one', () => {
    renderWithI18n(
      <QuantityStepper label="Telo mare" max={4} value={Number.NaN} onChange={() => {}} />,
    );
    expect(screen.getByRole('spinbutton', { name: 'Telo mare' })).toHaveTextContent('0');
  });

  it('reports the clamped value back when the bounds shrink below it', () => {
    const onChange = vi.fn();
    const { rerender } = renderWithI18n(
      <QuantityStepper label="Lettini" max={5} value={5} onChange={onChange} />,
    );
    expect(onChange).not.toHaveBeenCalled();
    rerender(<QuantityStepper label="Lettini" max={3} value={5} onChange={onChange} />);
    expect(onChange).toHaveBeenCalledWith(3);
    expect(screen.getByRole('spinbutton', { name: 'Lettini' })).toHaveTextContent('3');
  });

  it('ties the buttons to the live value', () => {
    renderWithI18n(<QuantityStepper label="Telo" max={4} value={1} onChange={() => {}} />);
    const value = screen.getByRole('spinbutton', { name: 'Telo' });
    expect(value).toHaveAttribute('aria-live', 'polite');
    expect(screen.getByRole('button', { name: 'Aumenta Telo' })).toHaveAttribute(
      'aria-controls',
      value.id,
    );
  });

  it('does nothing when disabled', async () => {
    const onChange = vi.fn();
    renderWithI18n(
      <QuantityStepper label="Cabina" max={1} value={0} disabled onChange={onChange} />,
    );
    const user = userEvent.setup();
    const value = screen.getByRole('spinbutton', { name: 'Cabina' });
    expect(value).toHaveAttribute('tabindex', '-1');
    expect(screen.getByRole('button', { name: 'Aumenta Cabina' })).toBeDisabled();
    value.focus();
    await user.keyboard('{ArrowUp}');
    expect(onChange).not.toHaveBeenCalled();
  });
});

describe('ExtraRow', () => {
  it('keeps the name on one line and the control at a fixed width (no overflow at 320 px)', () => {
    renderWithI18n(
      <div className="w-80">
        <ExtraList title="Extra">
          <ExtraRow
            name="Lettino aggiuntivo con materassino e cuscino"
            detail="per postazione · al giorno"
            hint="max 2"
            messageId="m1"
          >
            <QuantityStepper label="Lettino aggiuntivo" max={2} value={0} onChange={() => {}} />
          </ExtraRow>
        </ExtraList>
      </div>,
    );
    const name = screen.getByText('Lettino aggiuntivo con materassino e cuscino');
    // Full name on hover; truncated instead of wrapping.
    expect(name).toHaveAttribute('title', 'Lettino aggiuntivo con materassino e cuscino');
    expect(name).toHaveClass('truncate');
    expect(name.parentElement).toHaveClass('min-w-0', 'flex-1');
    const group = screen.getByRole('spinbutton').parentElement!;
    expect(group).toHaveClass('shrink-0');
    expect(group.parentElement).toHaveClass('shrink-0');
    expect(screen.getByRole('spinbutton')).toHaveClass('w-11');
    expect(screen.getByText('max 2')).toHaveAttribute('id', 'm1');
    expect(screen.getByRole('group', { name: 'Extra' }).tagName).toBe('FIELDSET');
  });

  it('shows the error on its own line instead of the hint', () => {
    renderWithI18n(
      <ul>
        <ExtraRow name="Telo mare" hint="max 4" error="Massimo 4." messageId="m2">
          <QuantityStepper
            label="Telo mare"
            max={4}
            value={4}
            aria-invalid
            aria-describedby="m2"
            onChange={() => {}}
          />
        </ExtraRow>
      </ul>,
    );
    expect(screen.getByRole('alert')).toHaveTextContent('Massimo 4.');
    expect(screen.getByRole('alert')).toHaveClass('basis-full');
    expect(screen.queryByText('max 4')).not.toBeInTheDocument();
    const value = screen.getByRole('spinbutton', { name: 'Telo mare' });
    expect(value).toHaveAttribute('aria-invalid', 'true');
    expect(value).toHaveAttribute('aria-describedby', 'm2');
    expect(value).toHaveAccessibleDescription('Massimo 4.');
  });
});
