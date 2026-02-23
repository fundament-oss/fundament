import {
  Component,
  ElementRef,
  ViewChild,
  AfterViewInit,
  Input,
  Output,
  EventEmitter,
  OnDestroy,
  ChangeDetectionStrategy,
  ViewEncapsulation,
} from '@angular/core';
import { Calendar } from 'vanilla-calendar-pro';
import 'vanilla-calendar-pro/styles/layout.css';

@Component({
  selector: 'app-date-range-picker',
  imports: [],
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  templateUrl: './date-range-picker.component.html',
  styleUrl: './date-range-picker.component.css',
})
export default class DateRangePickerComponent implements AfterViewInit, OnDestroy {
  @ViewChild('dateInput') dateInputRef!: ElementRef<HTMLInputElement>;

  @Input() id?: string;

  @Input() dateFrom = '';

  @Input() dateTo = '';

  @Output() dateFromChange = new EventEmitter<string>();

  @Output() dateToChange = new EventEmitter<string>();

  @Output() dateRangeChange = new EventEmitter<{ dateFrom: string; dateTo: string }>();

  private calendar?: Calendar;

  get displayValue(): string {
    if (this.dateFrom && this.dateTo) {
      if (this.dateFrom === this.dateTo) {
        return DateRangePickerComponent.formatDate(this.dateFrom);
      }
      return `${DateRangePickerComponent.formatDate(this.dateFrom)} - ${DateRangePickerComponent.formatDate(this.dateTo)}`;
    }
    return '';
  }

  ngAfterViewInit(): void {
    this.initCalendar();
  }

  ngOnDestroy(): void {
    if (this.calendar) {
      this.calendar.destroy();
    }
  }

  private static formatDate(dateStr: string): string {
    const date = new Date(dateStr);
    return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  }

  private initCalendar(): void {
    if (!this.dateInputRef) return;

    const initialDates = this.dateFrom && this.dateTo ? [this.dateFrom, this.dateTo] : [];

    this.calendar = new Calendar(this.dateInputRef.nativeElement, {
      type: 'multiple',
      selectionDatesMode: 'multiple-ranged',
      inputMode: true,
      positionToInput: 'auto',
      selectedDates: initialDates,
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      onClickDate: (self: any) => {
        const dates = self.context.selectedDates;
        if (dates && dates.length >= 2) {
          this.dateFrom = dates[0];
          this.dateTo = dates[1];
          this.dateFromChange.emit(this.dateFrom);
          this.dateToChange.emit(this.dateTo);
          this.dateRangeChange.emit({ dateFrom: this.dateFrom, dateTo: this.dateTo });
        }
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);

    this.calendar.init();
  }
}
