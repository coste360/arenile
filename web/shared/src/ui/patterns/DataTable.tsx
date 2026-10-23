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
import { Fragment, useId, useState, type ReactNode } from 'react';
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
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from '@arenile/shared/ui/table';
import { cn } from '@arenile/shared/lib/utils';

export type { ColumnDef } from '@tanstack/react-table';

/**
 * Column metadata understood by DataTable (`columnDef.meta`).
 * `align: 'right'` is for numbers and money: right-aligned, tabular figures,
 * in the header, the cells and the footer.
 */
export interface DataTableColumnMeta {
  /** Name in the column visibility menu. */
  label?: string;
  align?: 'left' | 'right' | 'center';
}

function columnMeta<TData>(column: Column<TData, unknown>): DataTableColumnMeta {
  return (column.columnDef.meta as DataTableColumnMeta | undefined) ?? {};
}

const ALIGN_CLASS = {
  left: '',
  right: 'text-right tabular-nums',
  center: 'text-center',
} as const;

function alignClass<TData>(column: Column<TData, unknown>): string {
  return ALIGN_CLASS[columnMeta(column).align ?? 'left'];
}

/** Server-side "load more" under the table (cursor-paginated lists). */
export interface DataTableLoadMore {
  hasMore: boolean;
  loading?: boolean;
  onLoadMore: () => void;
  /** Button text (default "Carica altri"). */
  label?: string;
}

export interface DataTableProps<TData, TValue = unknown> {
  columns: ColumnDef<TData, TValue>[];
  data: TData[];
  /** Accessible name of the table (rendered as an sr-only caption). */
  caption: string;
  /** Show the search field: a text filter over every column (global filter). */
  searchable?: boolean;
  searchPlaceholder?: string;
  /**
   * Controlled search for server-side filtering: the field shows `searchValue`
   * and reports changes; rows are not filtered client-side.
   */
  searchValue?: string;
  onSearchChange?: (value: string) => void;
  /** Filters next to the search field (selects, toggles, date pickers), left-aligned. */
  filters?: ReactNode;
  /** Actions on the right of the toolbar (e.g. "Nuovo" inside a tab). */
  toolbar?: ReactNode;
  /** Column visibility menu (default true when there are > 3 columns). */
  columnToggle?: boolean;
  /** Rows per page; 0 disables pagination (default 20). */
  pageSize?: number;
  pageSizeOptions?: number[];
  /** Server-side pagination: a "Carica altri" button under the table. */
  loadMore?: DataTableLoadMore;
  /** Shown when there are no rows (e.g. an <EmptyState bordered={false}/>). */
  empty?: ReactNode;
  /** Skeleton rows while data is loading. */
  loading?: boolean;
  getRowId?: (row: TData, index: number) => string;
  /** Row click (also Enter on the focused row); keep a real link/button in a cell too. */
  onRowClick?: (row: TData) => void;
  /** Accessible name of a clickable row (e.g. "Apri A1, Rossi Giulia"). */
  getRowLabel?: (row: TData) => string;
  /**
   * Group rows under full-width heading rows. Groups keep the order of first
   * appearance in `data`; rows inside a group follow the current sorting.
   */
  groupBy?: (row: TData) => string;
  /** Content of a group heading row (default: the key). */
  renderGroup?: (key: string, rows: TData[]) => ReactNode;
  /**
   * Without its own border, for a table inside a <SectionCard flush>; the first
   * and last columns align with the card padding.
   */
  embedded?: boolean;
  initialSorting?: SortingState;
  initialColumnVisibility?: VisibilityState;
  className?: string;
}

/**
 * shadcn data-table pattern on TanStack Table: sorting, global filter,
 * pagination, column visibility, grouping, footer totals, loading and empty
 * states. Columns declare `meta: { label: 'Nome', align: 'right' }` and use
 * <DataTableColumnHeader> as `header` to become sortable; a column `footer`
 * renders a totals row. On narrow screens the table scrolls horizontally
 * inside its container.
 */
export function DataTable<TData, TValue = unknown>({
  columns,
  data,
  caption,
  searchable,
  searchPlaceholder,
  searchValue,
  onSearchChange,
  filters,
  toolbar,
  columnToggle,
  pageSize = 20,
  pageSizeOptions = [10, 20, 50, 100],
  loadMore,
  empty,
  loading,
  getRowId,
  onRowClick,
  getRowLabel,
  groupBy,
  renderGroup,
  embedded,
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
  const serverSearch = onSearchChange !== undefined;

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
    manualFiltering: serverSearch,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    ...(paginated ? { getPaginationRowModel: getPaginationRowModel() } : {}),
    initialState: { pagination: { pageSize: paginated ? pageSize : data.length || 1 } },
  });

  const showToggle = columnToggle ?? columns.length > 3;
  const rows = table.getRowModel().rows;
  const visibleColumns = table.getVisibleLeafColumns().length;
  const hasFooter = table
    .getFooterGroups()
    .some((g) => g.headers.some((h) => !h.isPlaceholder && h.column.columnDef.footer));
  const search = serverSearch ? (searchValue ?? '') : globalFilter;
  const setSearch = serverSearch ? onSearchChange : setGlobalFilter;
  const edge = embedded ? 'first:pl-6 last:pr-6' : 'first:pl-4 last:pr-4';

  const body = (list: Row<TData>[]) =>
    list.map((row) => (
      <DataRow
        key={row.id}
        row={row}
        onRowClick={onRowClick}
        label={getRowLabel?.(row.original)}
        edge={edge}
      />
    ));

  return (
    <div className={cn('min-w-0 space-y-3', className)}>
      {(searchable || filters || toolbar || showToggle) && (
        <div className={cn('flex flex-wrap items-center gap-2', embedded && 'px-6')}>
          {searchable && (
            <InputGroup className="w-full sm:max-w-xs">
              <InputGroupAddon>
                <SearchIcon aria-hidden="true" />
              </InputGroupAddon>
              <InputGroupInput
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={searchPlaceholder ?? t('ui.table.filter')}
                aria-label={searchPlaceholder ?? t('ui.table.filter')}
              />
            </InputGroup>
          )}
          {filters}
          <div className="ml-auto flex flex-wrap items-center gap-2">
            {toolbar}
            {showToggle && <DataTableViewOptions table={table} />}
          </div>
        </div>
      )}
      <div
        data-slot="data-table"
        className={cn(
          embedded ? 'border-t' : 'overflow-hidden rounded-xl border bg-card shadow-sm',
        )}
      >
        <Table>
          <caption className="sr-only">{caption}</caption>
          <TableHeader className="bg-muted/50">
            {table.getHeaderGroups().map((group) => (
              <TableRow key={group.id} className="hover:bg-transparent">
                {group.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    className={cn(edge, alignClass(header.column))}
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
            {loading ? (
              Array.from({ length: 5 }, (_, i) => (
                <TableRow key={`skeleton-${i}`}>
                  {Array.from({ length: visibleColumns }, (_, j) => (
                    <TableCell key={j} className={edge}>
                      <Skeleton className="h-4 w-full max-w-40" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length ? (
              groupBy ? (
                groupRows(table.getCoreRowModel().rows, rows, groupBy).map(([key, groupRows]) => (
                  <Fragment key={`group-${key}`}>
                    <TableRow className="bg-muted/30 hover:bg-muted/30">
                      <TableCell
                        colSpan={visibleColumns}
                        className={cn(edge, 'py-2 text-sm font-medium')}
                      >
                        {renderGroup
                          ? renderGroup(
                              key,
                              groupRows.map((r) => r.original),
                            )
                          : key}
                      </TableCell>
                    </TableRow>
                    {body(groupRows)}
                  </Fragment>
                ))
              ) : (
                body(rows)
              )
            ) : (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={visibleColumns} className="p-0">
                  {empty ?? (
                    <p className="py-10 text-center text-sm text-muted-foreground">
                      {t('ui.table.empty')}
                    </p>
                  )}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
          {hasFooter && !loading && rows.length > 0 && (
            <TableFooter>
              {table.getFooterGroups().map((group) => (
                <TableRow key={group.id} className="hover:bg-transparent">
                  {group.headers.map((header) => (
                    <TableCell key={header.id} className={cn(edge, alignClass(header.column))}>
                      {header.isPlaceholder
                        ? null
                        : flexRender(header.column.columnDef.footer, header.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableFooter>
          )}
        </Table>
      </div>
      {paginated && table.getPageCount() > 1 && (
        <DataTablePagination table={table} pageSizeOptions={pageSizeOptions} />
      )}
      {loadMore?.hasMore && (
        <div className="flex justify-center">
          <Button variant="outline" onClick={loadMore.onLoadMore} disabled={loadMore.loading}>
            {loadMore.loading
              ? t('ui.table.loadingMore')
              : (loadMore.label ?? t('ui.table.loadMore'))}
          </Button>
        </div>
      )}
    </div>
  );
}

/** Splits the visible rows into groups ordered by first appearance in the unsorted data. */
function groupRows<TData>(
  core: Row<TData>[],
  visible: Row<TData>[],
  groupBy: (row: TData) => string,
): [string, Row<TData>[]][] {
  const order = new Map<string, number>();
  for (const r of core) {
    const k = groupBy(r.original);
    if (!order.has(k)) order.set(k, order.size);
  }
  const groups = new Map<string, Row<TData>[]>();
  for (const r of visible) {
    const k = groupBy(r.original);
    groups.set(k, [...(groups.get(k) ?? []), r]);
  }
  return [...groups.entries()].sort(([a], [b]) => (order.get(a) ?? 0) - (order.get(b) ?? 0));
}

function DataRow<TData>({
  row,
  onRowClick,
  label,
  edge,
}: {
  row: Row<TData>;
  onRowClick?: (row: TData) => void;
  label?: string;
  edge: string;
}) {
  const clickable = !!onRowClick;
  return (
    <TableRow
      data-state={row.getIsSelected() ? 'selected' : undefined}
      className={
        clickable
          ? 'cursor-pointer focus-visible:bg-muted/50 focus-visible:outline-none'
          : undefined
      }
      tabIndex={clickable ? 0 : undefined}
      aria-label={label}
      onClick={clickable ? () => onRowClick(row.original) : undefined}
      onKeyDown={
        clickable
          ? (e) => {
              if (e.target !== e.currentTarget) return;
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onRowClick(row.original);
              }
            }
          : undefined
      }
    >
      {row.getVisibleCells().map((cell) => (
        <TableCell key={cell.id} className={cn(edge, alignClass(cell.column))}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </TableCell>
      ))}
    </TableRow>
  );
}

function columnLabel<TData>(column: Column<TData, unknown>): string {
  return columnMeta(column).label ?? column.id;
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
  const right = columnMeta(column as Column<TData, unknown>).align === 'right';
  if (!column.getCanSort() && !column.getCanHide()) {
    return <span className={className}>{title}</span>;
  }
  const sorted = column.getIsSorted();
  const Icon = sorted === 'asc' ? ArrowUpIcon : sorted === 'desc' ? ArrowDownIcon : ArrowUpDownIcon;
  return (
    <div className={cn('flex items-center', right && 'justify-end', className)}>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn('h-8 data-[state=open]:bg-accent', right ? '-mr-3' : '-ml-3')}
            aria-label={t('ui.table.sortBy', { column: title })}
          >
            {title}
            <Icon aria-hidden="true" className="text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align={right ? 'end' : 'start'}>
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
