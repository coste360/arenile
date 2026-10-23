import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type Column,
  type ColumnDef,
  type ColumnFiltersState,
  type Row,
  type SortingState,
  type Table as TanTable,
  type VisibilityState,
} from '@tanstack/react-table';
import {
  ArrowDownIcon,
  ArrowUpDownIcon,
  ArrowUpIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronsLeftIcon,
  ChevronsRightIcon,
  EyeOffIcon,
  SearchIcon,
  Settings2Icon,
} from 'lucide-react';
import { useId, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@arenile/shared/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@arenile/shared/ui/dropdown-menu';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@arenile/shared/ui/input-group';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@arenile/shared/ui/select';
import { Skeleton } from '@arenile/shared/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { cn } from '@arenile/shared/lib/utils';

export type { ColumnDef } from '@tanstack/react-table';

export interface DataTableProps<TData, TValue = unknown> {
  columns: ColumnDef<TData, TValue>[];
  data: TData[];
  /** Accessible name of the table (rendered as an sr-only caption). */
  caption: string;
  /** Show a text filter over every column (global filter). */
  searchable?: boolean;
  searchPlaceholder?: string;
  /** Extra controls in the toolbar (filters, "Nuovo" button...). */
  toolbar?: ReactNode;
  /** Column visibility menu (default true when there are > 3 columns). */
  columnToggle?: boolean;
  /** Rows per page; 0 disables pagination (default 20). */
  pageSize?: number;
  pageSizeOptions?: number[];
  /** Shown when there are no rows (e.g. an <EmptyState/>). */
  empty?: ReactNode;
  /** Skeleton rows while data is loading. */
  loading?: boolean;
  getRowId?: (row: TData, index: number) => string;
  /** Row click (also Enter on the focused row); keep a real link/button in a cell too. */
  onRowClick?: (row: TData) => void;
  initialSorting?: SortingState;
  initialColumnVisibility?: VisibilityState;
  className?: string;
}

/**
 * shadcn data-table pattern on TanStack Table: sorting, global filter,
 * pagination, column visibility, loading and empty states. Columns declare
 * `meta: { label: 'Nome' }` to name themselves in the visibility menu, and
 * use <DataTableColumnHeader> as `header` to become sortable.
 */
export function DataTable<TData, TValue = unknown>({
  columns,
  data,
  caption,
  searchable,
  searchPlaceholder,
  toolbar,
  columnToggle,
  pageSize = 20,
  pageSizeOptions = [10, 20, 50, 100],
  empty,
  loading,
  getRowId,
  onRowClick,
  initialSorting = [],
  initialColumnVisibility = {},
  className,
}: DataTableProps<TData, TValue>) {
  const { t } = useTranslation();
  const [sorting, setSorting] = useState<SortingState>(initialSorting);
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [globalFilter, setGlobalFilter] = useState('');
  const [columnVisibility, setColumnVisibility] = useState(initialColumnVisibility);
  const paginated = pageSize > 0;

  // TanStack Table returns non-memoizable functions; this is expected.
  // eslint-disable-next-line react-hooks/incompatible-library
  const table = useReactTable({
    data,
    columns,
    getRowId,
    state: { sorting, columnFilters, globalFilter, columnVisibility },
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    onGlobalFilterChange: setGlobalFilter,
    onColumnVisibilityChange: setColumnVisibility,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    ...(paginated ? { getPaginationRowModel: getPaginationRowModel() } : {}),
    initialState: { pagination: { pageSize: paginated ? pageSize : data.length || 1 } },
  });

  const showToggle = columnToggle ?? columns.length > 3;
  const rows = table.getRowModel().rows;
  const visibleColumns = table.getVisibleLeafColumns().length;

  return (
    <div className={cn('space-y-3', className)}>
      {(searchable || toolbar || showToggle) && (
        <div className="flex flex-wrap items-center gap-2">
          {searchable && (
            <InputGroup className="w-full sm:max-w-xs">
              <InputGroupAddon>
                <SearchIcon aria-hidden="true" />
              </InputGroupAddon>
              <InputGroupInput
                type="search"
                value={globalFilter}
                onChange={(e) => setGlobalFilter(e.target.value)}
                placeholder={searchPlaceholder ?? t('ui.table.filter')}
                aria-label={searchPlaceholder ?? t('ui.table.filter')}
              />
            </InputGroup>
          )}
          <div className="ml-auto flex flex-wrap items-center gap-2">
            {toolbar}
            {showToggle && <DataTableViewOptions table={table} />}
          </div>
        </div>
      )}
      <div className="overflow-hidden rounded-lg border bg-card">
        <Table>
          <caption className="sr-only">{caption}</caption>
          <TableHeader className="bg-muted/50">
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id} className="hover:bg-transparent">
                {group.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    aria-sort={
                      header.column.getIsSorted() === 'asc'
                        ? 'ascending'
                        : header.column.getIsSorted() === 'desc'
                          ? 'descending'
                          : undefined
                    }
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(header.column.columnDef.header, header.getContext())}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {loading
              ? Array.from({ length: 5 }, (_, i) => (
                  <TableRow key={`skeleton-${i}`}>
                    {Array.from({ length: visibleColumns }, (_, j) => (
                      <TableCell key={j}>
                        <Skeleton className="h-4 w-full max-w-40" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              : rows.map((row) => <DataRow key={row.id} row={row} onRowClick={onRowClick} />)}
          </TableBody>
        </Table>
        {/* The empty state sits outside the horizontally scrolling table, so
            at phone width it is never clipped by the columns' min width. */}
        {!loading && rows.length === 0 && (
          <div className="border-t" data-slot="table-empty">
            {empty ?? (
              <p className="py-10 text-center text-sm text-muted-foreground">
                {t('ui.table.empty')}
              </p>
            )}
          </div>
        )}
      </div>
      {paginated && table.getPageCount() > 1 && (
        <DataTablePagination table={table} pageSizeOptions={pageSizeOptions} />
      )}
    </div>
  );
}

function DataRow<TData>({
  row,
  onRowClick,
}: {
  row: Row<TData>;
  onRowClick?: (row: TData) => void;
}) {
  return (
    <TableRow
      data-state={row.getIsSelected() ? 'selected' : undefined}
      className={onRowClick ? 'cursor-pointer' : undefined}
      onClick={onRowClick ? () => onRowClick(row.original) : undefined}
    >
      {row.getVisibleCells().map((cell) => (
        <TableCell key={cell.id}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </TableCell>
      ))}
    </TableRow>
  );
}

function columnLabel<TData>(column: Column<TData, unknown>): string {
  const meta = column.columnDef.meta as { label?: string } | undefined;
  return meta?.label ?? column.id;
}

/** Sortable column header: `header: ({ column }) => <DataTableColumnHeader column={column} title="Nome" />`. */
export function DataTableColumnHeader<TData, TValue>({
  column,
  title,
  className,
}: {
  column: Column<TData, TValue>;
  title: string;
  className?: string;
}) {
  const { t } = useTranslation();
  if (!column.getCanSort() && !column.getCanHide()) {
    return <span className={className}>{title}</span>;
  }
  const sorted = column.getIsSorted();
  const Icon = sorted === 'asc' ? ArrowUpIcon : sorted === 'desc' ? ArrowDownIcon : ArrowUpDownIcon;
  return (
    <div className={cn('flex items-center', className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="-ml-3 h-8 data-[state=open]:bg-accent"
            aria-label={t('ui.table.sortBy', { column: title })}
          >
            {title}
            <Icon aria-hidden="true" className="text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {column.getCanSort() && (
            <>
              <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
                <ArrowUpIcon aria-hidden="true" />
                {t('ui.table.sortAsc')}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
                <ArrowDownIcon aria-hidden="true" />
                {t('ui.table.sortDesc')}
              </DropdownMenuItem>
            </>
          )}
          {column.getCanSort() && column.getCanHide() && <DropdownMenuSeparator />}
          {column.getCanHide() && (
            <DropdownMenuItem onClick={() => column.toggleVisibility(false)}>
              <EyeOffIcon aria-hidden="true" />
              {t('ui.table.hideColumn')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

/** Column visibility menu (used by DataTable; exported for custom toolbars). */
export function DataTableViewOptions<TData>({ table }: { table: TanTable<TData> }) {
  const { t } = useTranslation();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" aria-label={t('ui.table.toggleColumns')}>
          <Settings2Icon aria-hidden="true" />
          <span className="hidden sm:inline">{t('ui.table.columns')}</span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        <DropdownMenuLabel>{t('ui.table.toggleColumns')}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {table
          .getAllLeafColumns()
          .filter((c) => c.getCanHide())
          .map((column) => (
            <DropdownMenuCheckboxItem
              key={column.id}
              checked={column.getIsVisible()}
              onCheckedChange={(v) => column.toggleVisibility(!!v)}
            >
              {columnLabel(column)}
            </DropdownMenuCheckboxItem>
          ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Pager with rows-per-page select (used by DataTable). */
export function DataTablePagination<TData>({
  table,
  pageSizeOptions = [10, 20, 50, 100],
}: {
  table: TanTable<TData>;
  pageSizeOptions?: number[];
}) {
  const { t } = useTranslation();
  const { pageIndex, pageSize } = table.getState().pagination;
  const sizeLabelId = useId();
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
      <p>{t('ui.table.rowCount', { count: table.getFilteredRowModel().rows.length })}</p>
      <div className="flex flex-wrap items-center gap-4">
        <div className="flex items-center gap-2">
          <span id={sizeLabelId}>{t('ui.table.rowsPerPage')}</span>
          <Select value={String(pageSize)} onValueChange={(v) => table.setPageSize(Number(v))}>
            <SelectTrigger size="sm" className="w-20" aria-labelledby={sizeLabelId}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {pageSizeOptions.map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <span className="font-medium text-foreground">
          {t('ui.table.pageOf', { page: pageIndex + 1, pages: table.getPageCount() })}
        </span>
        <div className="flex items-center gap-1">
          <Button
            variant="outline"
            size="icon-sm"
            className="hidden sm:inline-flex"
            onClick={() => table.setPageIndex(0)}
            disabled={!table.getCanPreviousPage()}
            aria-label={t('ui.firstPage')}
          >
            <ChevronsLeftIcon aria-hidden="true" />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
            aria-label={t('ui.previousPage')}
          >
            <ChevronLeftIcon aria-hidden="true" />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
            aria-label={t('ui.nextPage')}
          >
            <ChevronRightIcon aria-hidden="true" />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            className="hidden sm:inline-flex"
            onClick={() => table.setPageIndex(table.getPageCount() - 1)}
            disabled={!table.getCanNextPage()}
            aria-label={t('ui.lastPage')}
          >
            <ChevronsRightIcon aria-hidden="true" />
          </Button>
        </div>
      </div>
    </div>
  );
}
