import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { I18nextProvider } from 'react-i18next';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { createAdminTestI18n } from '../../i18n';
import { KitPicker } from './KitPicker';

const renderWithI18n = (ui: ReactElement) =>
  render(<I18nextProvider i18n={createAdminTestI18n()}>{ui}</I18nextProvider>);

describe('KitPicker', () => {
  beforeAll(() => {
    // cmdk scrolls the highlighted item into view; jsdom has no layout.
    Element.prototype.scrollIntoView = vi.fn();
  });

  const selectedId = () =>
    screen
      .getByRole('listbox')
      .querySelector('[cmdk-item][aria-selected="true"]')
      ?.getAttribute('data-asset');

  it('lists bookable assets by category, marks and announces the current one', () => {
    const onPick = vi.fn();
    renderWithI18n(
      <KitPicker family="spots" label="Illustrazione" value="gazebo" onPick={onPick} />,
    );
    const list = screen.getByRole('listbox');
    expect(within(list).getByText('Postazioni')).toBeInTheDocument();
    expect(list.querySelector('[data-asset="bar"]')).toBeNull();
    const current = list.querySelector('[data-asset="gazebo"][data-current]') as HTMLElement;
    expect(current).toHaveTextContent('(illustrazione attuale)');
    fireEvent.click(list.querySelector('[data-asset="lettino"]') as HTMLElement);
    expect(onPick).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'lettino', spotKind: 'sunbed' }),
    );
  });

  it('moves one step with Left/Right and one row (four items) with Up/Down', async () => {
    const user = userEvent.setup();
    renderWithI18n(<KitPicker family="spots" label="Illustrazione" onPick={() => undefined} />);
    const items = [...screen.getByRole('listbox').querySelectorAll('[cmdk-item]')].map((i) =>
      i.getAttribute('data-asset'),
    );
    await user.click(screen.getByPlaceholderText("Cerca un'illustrazione…"));
    await user.keyboard('{ArrowRight}');
    expect(selectedId()).toBe(items[1]);
    await user.keyboard('{ArrowDown}');
    expect(selectedId()).toBe(items[5]);
    await user.keyboard('{ArrowLeft}');
    expect(selectedId()).toBe(items[4]);
    await user.keyboard('{ArrowUp}');
    expect(selectedId()).toBe(items[0]);
  });
});
