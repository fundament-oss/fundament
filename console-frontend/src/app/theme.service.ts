import { computed, Injectable, signal } from '@angular/core';

function loadStoredTheme(): 'default' | 'overheid' {
  return localStorage.getItem('ui-theme') === 'overheid' ? 'overheid' : 'default';
}

@Injectable({ providedIn: 'root' })
export default class ThemeService {
  readonly activeUiTheme = signal<'default' | 'overheid'>(loadStoredTheme());

  readonly isOverheidTheme = computed(() => this.activeUiTheme() === 'overheid');

  toggleUiTheme() {
    const next = this.isOverheidTheme() ? 'default' : 'overheid';
    this.activeUiTheme.set(next);
    localStorage.setItem('ui-theme', next);
  }
}
