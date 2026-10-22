import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
  Input,
  Label,
} from './index';

describe('ui primitives', () => {
  it('Button defaults to type=button and supports asChild', () => {
    render(
      <>
        <Button>Salva</Button>
        <Button asChild>
          <a href="/x">Vai</a>
        </Button>
      </>,
    );
    expect(screen.getByRole('button', { name: 'Salva' })).toHaveAttribute('type', 'button');
    expect(screen.getByRole('link', { name: 'Vai' })).toHaveClass('inline-flex');
  });

  it('Label is associated with Input', () => {
    render(
      <>
        <Label htmlFor="e">Email</Label>
        <Input id="e" />
      </>,
    );
    expect(screen.getByLabelText('Email')).toBeInTheDocument();
  });

  it('Dialog opens and closes', async () => {
    const user = userEvent.setup();
    render(
      <Dialog>
        <DialogTrigger>Apri</DialogTrigger>
        <DialogContent closeLabel="Chiudi">
          <DialogTitle>Titolo</DialogTitle>
          <DialogDescription>Descrizione</DialogDescription>
        </DialogContent>
      </Dialog>,
    );
    await user.click(screen.getByRole('button', { name: 'Apri' }));
    expect(screen.getByRole('dialog', { name: 'Titolo' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Chiudi' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
