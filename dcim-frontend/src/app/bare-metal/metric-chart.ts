import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  input,
  viewChild,
} from '@angular/core';
import { Chart, type ChartConfiguration, type ChartDataset, registerables } from 'chart.js';
import ThemeService from '../theme.service';
import type { ChartSeries } from './metal-metrics';

Chart.register(...registerables);

/**
 * The design system's colour primitives, step 600 of lintblauw, mintgroen,
 * oranje, rood, paars, coolgray and groen. A canvas cannot resolve a
 * `light-dark()` token, so each scheme carries the hex value that token takes in
 * that scheme (from `colors.generated.css`). Step 600 keeps every series above
 * 4.5:1 against the base surface in both schemes.
 */
const LIGHT = ['#366396', '#006E58', '#9B4A00', '#B62C20', '#794E9B', '#556273', '#326E15'];
const DARK = ['#608FC5', '#3D9B82', '#DB6D00', '#EA5E4D', '#A679CA', '#7F8DA0', '#5D9B45'];

/** Axis text: `--primitives-color-coolgray-650`. */
const TEXT = { light: '#4D5867', dark: '#8C99AA' };

/** Grid lines: `--primitives-color-coolgray-200`. */
const GRID = { light: '#B7C0CC', dark: '#333A45' };

type ChartKind = 'line' | 'bar';

/**
 * One chart plus what a canvas cannot say: an HTML legend with text labels and
 * a `<details>` table of the same numbers, so the data is not locked inside
 * pixels. Charts are rebuilt rather than mutated, which is also what a theme
 * change needs.
 */
@Component({
  selector: 'app-metric-chart',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <figure class="m-0">
      @if (series().length > 1) {
        <ul
          class="mb-2 flex flex-wrap gap-x-4 gap-y-1 text-body-sm text-content-secondary"
          aria-label="Legend"
        >
          @for (item of legend(); track item.label) {
            <li class="flex items-center gap-2">
              <span
                class="inline-block h-2.5 w-2.5 rounded-sm"
                [style.background-color]="item.color"
                aria-hidden="true"
              ></span>
              {{ item.label }}
            </li>
          }
        </ul>
      }
      <div class="relative w-full" [class.h-64]="!horizontal()" [class.h-80]="horizontal()">
        <canvas #canvas role="img" [attr.aria-label]="accessibleLabel()"></canvas>
      </div>
      <details class="mt-2">
        <summary class="cursor-pointer text-body-sm text-content-secondary">Show data</summary>
        <div class="mt-2 max-h-64 overflow-auto">
          <table class="w-full border-collapse text-body-sm">
            <caption class="sr-only">
              {{
                accessibleLabel()
              }}
            </caption>
            <thead>
              <tr>
                <th scope="col" class="px-2 py-1 text-left font-semibold">{{ xLabel() }}</th>
                @for (item of series(); track item.label) {
                  <th scope="col" class="px-2 py-1 text-right font-semibold">
                    {{ item.label }}
                  </th>
                }
              </tr>
            </thead>
            <tbody>
              @for (label of labels(); track $index) {
                <tr class="border-t border-dividers">
                  <th scope="row" class="px-2 py-1 text-left font-normal">{{ label }}</th>
                  @for (item of series(); track item.label) {
                    <td class="px-2 py-1 text-right">{{ valueText(item.values[$index]) }}</td>
                  }
                </tr>
              }
            </tbody>
          </table>
        </div>
      </details>
    </figure>
  `,
})
export default class MetricChartComponent {
  readonly kind = input<ChartKind>('line');

  readonly labels = input.required<string[]>();

  readonly series = input.required<ChartSeries[]>();

  readonly stacked = input(false);

  readonly horizontal = input(false);

  readonly accessibleLabel = input.required<string>();

  readonly xLabel = input('Time');

  readonly format = input<(value: number) => string>((value) => String(value));

  private readonly theme = inject(ThemeService);

  private readonly canvas = viewChild<ElementRef<HTMLCanvasElement>>('canvas');

  protected readonly legend = computed(() => {
    const palette = this.theme.isDarkMode() ? DARK : LIGHT;
    return this.series().map((item, index) => ({
      label: item.label,
      color: palette[index % palette.length],
    }));
  });

  constructor() {
    effect((onCleanup) => {
      const canvas = this.canvas()?.nativeElement;
      if (!canvas) return;
      const chart = new Chart(canvas, this.configuration());
      onCleanup(() => chart.destroy());
    });
  }

  /** A table cell, through the same formatter the chart's axis uses. */
  protected valueText(value: number | undefined): string {
    return this.format()(value ?? 0);
  }

  private configuration(): ChartConfiguration {
    const dark = this.theme.isDarkMode();
    const palette = dark ? DARK : LIGHT;
    const text = dark ? TEXT.dark : TEXT.light;
    const grid = dark ? GRID.dark : GRID.light;
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const horizontal = this.horizontal();
    const stacked = this.stacked();
    const format = this.format();

    const datasets: ChartDataset[] = this.series().map((item, index) => ({
      label: item.label,
      data: item.values,
      backgroundColor: palette[index % palette.length],
      borderColor: palette[index % palette.length],
      borderWidth: this.kind() === 'line' ? 2 : 0,
      borderRadius: this.kind() === 'bar' ? 3 : 0,
      pointRadius: 0,
      pointHitRadius: 8,
      tension: 0.3,
      fill: false,
    }));

    const category = {
      stacked,
      grid: { display: false },
      ticks: { color: text, autoSkip: true, maxTicksLimit: horizontal ? 20 : 8 },
    };
    const value = {
      stacked,
      beginAtZero: true,
      grid: { color: grid },
      border: { display: false },
      ticks: { color: text, callback: (tick: string | number) => format(Number(tick)) },
    };

    return {
      type: this.kind(),
      data: { labels: this.labels(), datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        indexAxis: horizontal ? 'y' : 'x',
        animation: reduceMotion ? false : { duration: 150 },
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              label: (item) => `${item.dataset.label ?? ''}: ${format(Number(item.raw))}`,
            },
          },
        },
        scales: horizontal ? { x: value, y: category } : { x: category, y: value },
      },
    };
  }
}
