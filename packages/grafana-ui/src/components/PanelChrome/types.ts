/**
 * Mode to describe if a legend is isolated/selected or being appended to an existing
 * series selection.
 * @alpha
 */

export enum SeriesVisibilityChangeMode {
  ToggleSelection = 'select',
  AppendToSelection = 'append',
}

export type OnSelectCallback = (selections: Array<{ key: string; from: number; to: number }>) => void;
