import { Injectable, computed, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Title } from '@angular/platform-browser';
import { Slide, Tour } from './presentation.model';
import { DEFAULT_TOUR_ID, PERSONA_TOURS, STORY_TOURS, TOURS } from './tours';
import { runDrive } from './drive-runner';
import { closeOpenAppDialogs } from './app-dialogs';

/**
 * Drives the walkthrough overlay: slide state, URL sync (present/tour/slide query
 * params), navigation of the app pane, the `presenting` html classes, and auto-drive.
 * Provided in root but only ever activated in the demo build.
 */
@Injectable({ providedIn: 'root' })
export class PresentationService {
  private readonly router = inject(Router);

  private readonly title = inject(Title);

  readonly active = signal(false);

  /** `chooser` shows the tour/persona picker; `tour` shows the narration panel. */
  readonly mode = signal<'chooser' | 'tour'>('chooser');

  readonly storyTours = STORY_TOURS;

  readonly personaTours = PERSONA_TOURS;

  private readonly tourId = signal<string>(DEFAULT_TOUR_ID);

  readonly index = signal(0);

  readonly autoplay = signal(false);

  readonly skipOptional = signal(false);

  /** Whether the browser is in native fullscreen (toggled with `f`). */
  readonly browserFullscreen = signal(false);

  readonly tour = computed<Tour>(() => TOURS[this.tourId()] ?? TOURS[DEFAULT_TOUR_ID]);

  readonly total = computed(() => this.tour().slides.length);

  readonly currentSlide = computed<Slide | undefined>(() => this.tour().slides[this.index()]);

  /** Full-bleed slide (opening/closing) — hides the app; unrelated to browser fullscreen. */
  readonly isFull = computed(() => !!this.currentSlide()?.full);

  /** Whether the panel fills the viewport: full-bleed slides and the chooser. */
  readonly deckFull = computed(() => this.mode() === 'chooser' || this.isFull());

  readonly progress = computed(() => ((this.index() + 1) / this.total()) * 100);

  /** Current slide number, zero-padded to the width of the total (e.g. "05"). */
  readonly currentLabel = computed(() =>
    String(this.index() + 1).padStart(String(this.total()).length, '0'),
  );

  private driveController: AbortController | null = null;

  private autoplayTimer: ReturnType<typeof setInterval> | null = null;

  private static readonly AUTOPLAY_MS = 6000;

  constructor() {
    document.addEventListener('fullscreenchange', () => {
      this.browserFullscreen.set(!!document.fullscreenElement);
    });
  }

  /**
   * Reads present/tour/slide from the current URL and starts the walkthrough.
   * The demo build presents by default; pass `?present=0` to open the plain console.
   * Without a `tour` param it opens the default tour at its first slide; the chooser
   * is reached from there via "Naar de keuze". `?tour=<id>` deep-links into a tour.
   */
  initFromUrl(): void {
    const params = new URLSearchParams(window.location.search);
    if (params.get('present') === '0') return;
    const tourId = params.get('tour');
    if (!tourId) {
      this.startTour(DEFAULT_TOUR_ID);
      return;
    }
    const slide = Math.max(1, parseInt(params.get('slide') || '1', 10)) - 1;
    this.startTour(tourId, slide);
  }

  startTour(tourId: string, index = 0): void {
    this.tourId.set(TOURS[tourId] ? tourId : DEFAULT_TOUR_ID);
    this.active.set(true);
    this.mode.set('tour');
    this.goto(index);
  }

  /** Leave the current tour for the picker, keeping the presentation open. */
  backToChooser(): void {
    this.cancelDrive();
    this.stopAutoplay();
    this.showChooser();
  }

  private showChooser(): void {
    closeOpenAppDialogs();
    this.active.set(true);
    this.mode.set('chooser');
    this.applyClasses();
    this.applyTitle();
    // Drop tour/slide so a reload lands on the chooser again.
    this.router.navigate([this.currentPath()], { queryParams: { present: 1 } });
  }

  goto(index: number): void {
    // An open app modal (native <dialog>) traps focus and makes the deck inert, so
    // close it before moving on — otherwise the presenter is stuck on the slide.
    closeOpenAppDialogs();
    const clamped = Math.min(Math.max(0, index), this.total() - 1);
    this.index.set(clamped);
    this.applyClasses();
    this.applyTitle();
    this.syncUrlAndNavigate();
  }

  /**
   * While presenting, the document title is the slide title. Route components still
   * call TitleService.setTitle() on init, but the demo build's DemoTitleService
   * ignores those calls while active, so this value sticks.
   */
  private applyTitle(): void {
    if (this.mode() === 'chooser') {
      this.title.setTitle('Fundament — kies je rondleiding');
      return;
    }
    const slide = this.currentSlide();
    if (slide) this.title.setTitle(slide.title);
  }

  next(): void {
    const target = this.nextIndex(this.index(), 1);
    if (target !== this.index()) this.goto(target);
  }

  prev(): void {
    const target = this.nextIndex(this.index(), -1);
    if (target !== this.index()) this.goto(target);
  }

  /** Next index in `dir`, skipping `skippable` slides when skipOptional is on. */
  private nextIndex(from: number, dir: 1 | -1): number {
    let i = from + dir;
    while (i > 0 && i < this.total() - 1 && this.skipOptional() && this.tour().slides[i].skippable) {
      i += dir;
    }
    return Math.min(Math.max(0, i), this.total() - 1);
  }

  /** Toggle the browser's native fullscreen (the `f` shortcut). */
  toggleFull(): void {
    if (document.fullscreenElement) {
      void document.exitFullscreen().catch(() => undefined);
    } else {
      void document.documentElement.requestFullscreen().catch(() => undefined);
    }
  }

  toggleSkipOptional(): void {
    this.skipOptional.update((v) => !v);
  }

  toggleAutoplay(): void {
    if (this.autoplayTimer) {
      this.stopAutoplay();
      return;
    }
    this.autoplay.set(true);
    this.autoplayTimer = setInterval(() => {
      if (this.index() >= this.total() - 1) {
        this.stopAutoplay();
        return;
      }
      this.next();
    }, PresentationService.AUTOPLAY_MS);
  }

  private stopAutoplay(): void {
    if (this.autoplayTimer) clearInterval(this.autoplayTimer);
    this.autoplayTimer = null;
    this.autoplay.set(false);
  }

  stop(): void {
    this.cancelDrive();
    this.stopAutoplay();
    closeOpenAppDialogs();
    if (document.fullscreenElement) void document.exitFullscreen().catch(() => undefined);
    this.active.set(false);
    this.mode.set('chooser');
    document.documentElement.classList.remove('presenting', 'presenting-full');
    // Hand the title back to the console; the next route change re-sets it.
    this.title.setTitle('Fundament Console');
    this.router.navigate([this.currentPath()], { queryParams: {} });
  }

  /** The chooser covers the whole viewport, like a full-bleed slide. */
  private applyClasses(): void {
    const root = document.documentElement.classList;
    root.toggle('presenting', this.active());
    root.toggle('presenting-full', this.active() && (this.isFull() || this.mode() === 'chooser'));
  }

  private currentPath(): string {
    return this.router.url.split('?')[0] || '/';
  }

  private syncUrlAndNavigate(): void {
    this.cancelDrive();
    const slide = this.currentSlide();
    const queryParams = { present: 1, tour: this.tourId(), slide: this.index() + 1 };
    const path = slide?.route ?? this.currentPath();
    this.router.navigate([path], { queryParams }).then(() => {
      if (slide?.drive?.length) this.startDrive(slide);
    });
  }

  private startDrive(slide: Slide): void {
    this.cancelDrive();
    const controller = new AbortController();
    this.driveController = controller;
    void runDrive(slide.drive ?? [], controller.signal);
  }

  private cancelDrive(): void {
    this.driveController?.abort();
    this.driveController = null;
  }
}

