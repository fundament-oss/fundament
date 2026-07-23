import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  inject,
  input,
  model,
  output,
  signal,
} from '@angular/core';

interface CalendarDay {
  date: string;
  label: string;
  inMonth: boolean;
  isToday: boolean;
}

@Component({
  selector: 'app-date-range-picker',
  imports: [],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './date-range-picker.component.html',
  host: {
    '(document:click)': 'onDocumentClick($event)',
  },
})
export default class DateRangePickerComponent {
  private readonly el = inject(ElementRef);

  readonly id = input<string>();

  readonly dateFrom = model('');

  readonly dateTo = model('');

  readonly dateRangeChange = output<{ dateFrom: string; dateTo: string }>();

  readonly open = signal(false);

  readonly viewYear = signal(new Date().getFullYear());

  readonly viewMonth = signal(new Date().getMonth());

  readonly hoverDate = signal<string | null>(null);

  readonly weekdays = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa'];

  readonly displayValue = computed(() => {
    const from = this.dateFrom();
    const to = this.dateTo();
    if (from && to) {
      if (from === to) return DateRangePickerComponent.formatDate(from);
      return `${DateRangePickerComponent.formatDate(from)} – ${DateRangePickerComponent.formatDate(to)}`;
    }
    if (from) return DateRangePickerComponent.formatDate(from);
    return '';
  });

  readonly monthLabel = computed(() => {
    const d = new Date(this.viewYear(), this.viewMonth(), 1);
    return d.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
  });

  readonly weeks = computed((): CalendarDay[] => {
    const year = this.viewYear();
    const month = this.viewMonth();
    const todayStr = DateRangePickerComponent.toISO(new Date());

    const firstDay = new Date(year, month, 1);
    const startDate = new Date(firstDay);
    startDate.setDate(startDate.getDate() - firstDay.getDay());

    return Array.from({ length: 6 * 7 }, (_, i) => {
      const d = new Date(startDate);
      d.setDate(startDate.getDate() + i);
      const dateStr = DateRangePickerComponent.toISO(d);
      return {
        date: dateStr,
        label: String(d.getDate()),
        inMonth: d.getMonth() === month,
        isToday: dateStr === todayStr,
      };
    });
  });

  toggleOpen(): void {
    if (this.open()) {
      this.open.set(false);
      return;
    }
    const from = this.dateFrom();
    const candidate = from ? new Date(`${from}T00:00:00`) : null;
    const ref = candidate && !Number.isNaN(candidate.getTime()) ? candidate : new Date();
    this.viewYear.set(ref.getFullYear());
    this.viewMonth.set(ref.getMonth());
    this.open.set(true);
  }

  prevMonth(): void {
    if (this.viewMonth() === 0) {
      this.viewMonth.set(11);
      this.viewYear.update((y) => y - 1);
    } else {
      this.viewMonth.update((m) => m - 1);
    }
  }

  nextMonth(): void {
    if (this.viewMonth() === 11) {
      this.viewMonth.set(0);
      this.viewYear.update((y) => y + 1);
    } else {
      this.viewMonth.update((m) => m + 1);
    }
  }

  selectDate(date: string): void {
    const from = this.dateFrom();
    const to = this.dateTo();

    if (!from || to) {
      this.dateFrom.set(date);
      this.dateTo.set('');
      return;
    }

    let finalFrom = from;
    let finalTo = date;
    if (date < from) {
      finalFrom = date;
      finalTo = from;
    }
    this.dateFrom.set(finalFrom);
    this.dateTo.set(finalTo);
    this.dateRangeChange.emit({ dateFrom: finalFrom, dateTo: finalTo });
    this.hoverDate.set(null);
    this.open.set(false);
  }

  dayClass(day: CalendarDay): string {
    const from = this.dateFrom();
    const to = this.dateTo();
    const hover = this.hoverDate();
    const { date, inMonth, isToday } = day;

    const base = 'flex h-8 w-8 items-center justify-center rounded-full text-sm transition-colors';

    if (date === from || date === to) {
      return `${base} bg-accent-500 text-white`;
    }

    let rangeStart = from;
    let rangeEnd = to;
    if (from && !to && hover) {
      rangeStart = from <= hover ? from : hover;
      rangeEnd = from <= hover ? hover : from;
    }
    if (rangeStart && rangeEnd && date > rangeStart && date < rangeEnd) {
      return `${base} bg-accent-100 text-accent-900 dark:bg-accent-800/50 dark:text-accent-100`;
    }

    if (!inMonth) {
      return `${base} text-neutral-400 dark:text-neutral-600 hover:bg-neutral-100 dark:hover:bg-neutral-800`;
    }
    if (isToday) {
      return `${base} font-semibold text-accent-500 dark:text-accent-400 hover:bg-neutral-100 dark:hover:bg-neutral-800`;
    }
    return `${base} text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800`;
  }

  onDocumentClick(event: MouseEvent): void {
    if (!this.el.nativeElement.contains(event.target as Node)) {
      this.open.set(false);
    }
  }

  private static formatDate(dateStr: string): string {
    const [year, month, day] = dateStr.split('-').map(Number);
    return new Date(year, month - 1, day).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    });
  }

  private static toISO(date: Date): string {
    const y = date.getFullYear();
    const m = String(date.getMonth() + 1).padStart(2, '0');
    const d = String(date.getDate()).padStart(2, '0');
    return `${y}-${m}-${d}`;
  }
}
