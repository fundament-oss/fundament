/**
 * English strings for the design system components on screen here.
 *
 * The design system is Dutch first: every component ships Dutch defaults and
 * only takes an override through its own `translations` property. There is no
 * app-level language, so an English console has to hand one to every instance
 * that carries user-facing text. Keeping them here means the wording is decided
 * once instead of per template.
 */

const datePickerTranslations = {
  'components.date-picker.view-previous-month-action': 'Previous month',
  'components.date-picker.view-next-month-action': 'Next month',
  'components.date-picker.view-today-action': 'Today',
  'components.date-picker.choose-month-action': 'Choose a month',
  'components.date-picker.choose-year-action': 'Choose a year',
  'components.date-picker.january-lowercase': 'January',
  'components.date-picker.january-capitalize': 'January',
  'components.date-picker.february-lowercase': 'February',
  'components.date-picker.february-capitalize': 'February',
  'components.date-picker.march-lowercase': 'March',
  'components.date-picker.march-capitalize': 'March',
  'components.date-picker.april-lowercase': 'April',
  'components.date-picker.april-capitalize': 'April',
  'components.date-picker.may-lowercase': 'May',
  'components.date-picker.may-capitalize': 'May',
  'components.date-picker.june-lowercase': 'June',
  'components.date-picker.june-capitalize': 'June',
  'components.date-picker.july-lowercase': 'July',
  'components.date-picker.july-capitalize': 'July',
  'components.date-picker.august-lowercase': 'August',
  'components.date-picker.august-capitalize': 'August',
  'components.date-picker.september-lowercase': 'September',
  'components.date-picker.september-capitalize': 'September',
  'components.date-picker.october-lowercase': 'October',
  'components.date-picker.october-capitalize': 'October',
  'components.date-picker.november-lowercase': 'November',
  'components.date-picker.november-capitalize': 'November',
  'components.date-picker.december-lowercase': 'December',
  'components.date-picker.december-capitalize': 'December',
  'components.date-picker.sunday-lowercase': 'Sunday',
  'components.date-picker.monday-lowercase': 'Monday',
  'components.date-picker.tuesday-lowercase': 'Tuesday',
  'components.date-picker.wednesday-lowercase': 'Wednesday',
  'components.date-picker.thursday-lowercase': 'Thursday',
  'components.date-picker.friday-lowercase': 'Friday',
  'components.date-picker.saturday-lowercase': 'Saturday',
  'components.date-picker.sunday-short-lowercase': 'Su',
  'components.date-picker.monday-short-lowercase': 'Mo',
  'components.date-picker.tuesday-short-lowercase': 'Tu',
  'components.date-picker.wednesday-short-lowercase': 'We',
  'components.date-picker.thursday-short-lowercase': 'Th',
  'components.date-picker.friday-short-lowercase': 'Fr',
  'components.date-picker.saturday-short-lowercase': 'Sa',
  'components.date-picker.week-number-column-label': 'Week number',
  'components.date-picker.week-number-column-short-label': 'Wk',
  'components.date-picker.week-number-label': 'Week {week}',
  'components.date-picker.date-label': '{weekday} {month} {day}, {year}',
  'components.date-picker.today-lowercase': 'today',
  'components.date-picker.unavailable-lowercase-label': 'unavailable',
  'components.date-picker.range-anchor-lowercase-label': 'selected, period not complete yet',
  'components.date-picker.range-start-lowercase-label': 'start of the period',
  'components.date-picker.range-end-lowercase-label': 'end of the period',
  'components.date-picker.in-range-lowercase-label': 'in the period',
  'components.date-picker.date-selected-text': 'Selected: {date}.',
  'components.date-picker.range-anchor-text':
    'Selected: {date}. Now choose a second date, earlier or later.',
  'components.date-picker.range-selected-text': 'Selected: {start} through {end}.',
  'components.date-picker.range-blocked-text':
    'That period contains a date that is unavailable. Choose another.',
};

export default datePickerTranslations;
