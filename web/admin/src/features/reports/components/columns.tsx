import { DataTableColumnHeader, type ColumnDef } from '@arenile/shared/ui/patterns';

/** Sortable text column (the group label of a report row). */
export function textColumn<T>(id: string, title: string, get: (row: T) => string): ColumnDef<T> {
  return {
    id,
    accessorFn: get,
    meta: { label: title },
    header: ({ column }) => <DataTableColumnHeader column={column} title={title} />,
    cell: ({ row }) => <span className="font-medium">{get(row.original)}</span>,
  };
}

/**
 * Sortable numeric column, right aligned with tabular figures. Sorting uses
 * the raw integer (cents, basis points, counts), display uses `format`.
 */
export function numberColumn<T>(
  id: string,
  title: string,
  get: (row: T) => number,
  format: (value: number) => string,
): ColumnDef<T> {
  return {
    id,
    accessorFn: get,
    meta: { label: title },
    sortingFn: 'basic',
    header: ({ column }) => (
      <DataTableColumnHeader column={column} title={title} className="justify-end" />
    ),
    cell: ({ row }) => (
      <span className="block text-right whitespace-nowrap tabular-nums">
        {format(get(row.original))}
      </span>
    ),
  };
}
