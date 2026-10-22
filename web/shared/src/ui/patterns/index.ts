/**
 * Arenile building blocks composed from shadcn/ui components.
 * See ../README.md for usage. Import from '@arenile/shared/ui/patterns'.
 */
export { PageHeader, type PageHeaderProps } from './PageHeader';
export { EmptyState, type EmptyStateProps } from './EmptyState';
export { ConfirmDialog, type ConfirmDialogProps } from './ConfirmDialog';
export {
  DataTable,
  DataTableColumnHeader,
  DataTablePagination,
  DataTableViewOptions,
  type ColumnDef,
  type DataTableProps,
} from './DataTable';
export { FieldRow, FormSection, type FieldRowProps, type FormSectionProps } from './FormSection';
export { MoneyInput, PercentInput, type ScaledInputProps } from './MoneyInput';
export {
  DatePicker,
  DateRangePicker,
  type DatePickerProps,
  type DateRangePickerProps,
  type IsoDateRange,
} from './DatePicker';
export {
  I18N_TEXT_LOCALES,
  I18nTextInput,
  type I18nText,
  type I18nTextInputProps,
} from './I18nTextInput';
export { StatusBadge, type StatusBadgeProps, type StatusTone } from './StatusBadge';
export {
  COLOR_SCHEMES,
  ThemeProvider,
  ThemeRadioItems,
  ThemeToggle,
  useTheme,
  type ColorScheme,
  type ResolvedColorScheme,
  type ThemeProviderProps,
} from './ThemeProvider';
