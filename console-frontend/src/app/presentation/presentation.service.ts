import { Injectable, computed, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Title } from '@angular/platform-browser';
import { Slide, Tour } from './presentation.model';
import {
  DECK_NAME,
  DEFAULT_LOCALE,
  isLocale,
  Locale,
  Localized,
  LOCALE_STORAGE_KEY,
  UI,
} from './i18n';
import { DEFAULT_TOUR_ID, PERSONA_TOURS, STORY_TOURS, TOURS } from './tours';
import {
  EMBED_NAVIGATE_MESSAGE,
  EMBED_READY_MESSAGE,
  MARKETPLACE_EMBED_BASE,
} from './presentation.tokens';
import runDrive from './drive-runner';
import { closeOpenAppDialogs } from './app-dialogs';
import { ToastService } from '../toast.service';

/** `?lang` wins (shareable deep links), then the last choice, then Dutch. */
function resolveLocale(fromUrl: string | null): Locale {
  if (isLocale(fromUrl)) return fromUrl;
  const stored = localStorage.getItem(LOCALE_STORAGE_KEY);
  return isLocale(stored) ? stored : DEFAULT_LOCALE;
}

/**
 * Drives the walkthrough overlay: slide state, URL sync (present/tour/slide query
 * params), navigation of the app pane, the `presenting` html classes, and auto-drive.
 * Provided in root but only ever activated in the demo build.
 */
@Injectable({ providedIn: 'root' })
export default class PresentationService {
  private readonly router = inject(Router);

  private readonly title = inject(Title);

  private readonly toasts = inject(ToastService);

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

  /** Narration language (toggled with `l`); the app pane is unaffected. */
  readonly locale = signal<Locale>(DEFAULT_LOCALE);

  /** The overlay's own chrome in the current locale. */
  readonly ui = computed(() => UI[this.locale()]);

  readonly tour = computed<Tour>(() => TOURS[this.tourId()] ?? TOURS[DEFAULT_TOUR_ID]);

  readonly total = computed(() => this.tour().slides.length);

  readonly currentSlide = computed<Slide | undefined>(() => this.tour().slides[this.index()]);

  /** Full-bleed slide (opening/closing) — hides the app; unrelated to browser fullscreen. */
  readonly isFull = computed(() => !!this.currentSlide()?.full);

  /**
   * Path inside the embedded marketplace demo for this slide, or '' when the app
   * pane shows the console. The overlay renders the frame off this.
   */
  readonly embedPath = computed(() => (this.mode() === 'tour' && this.currentSlide()?.embed) || '');

  /** Whether the panel fills the viewport: full-bleed slides and the chooser. */
  readonly deckFull = computed(() => this.mode() === 'chooser' || this.isFull());

  readonly progress = computed(() => ((this.index() + 1) / this.total()) * 100);

  /** Current slide number, zero-padded to the width of the total (e.g. "05"). */
  readonly currentLabel = computed(() =>
    String(this.index() + 1).padStart(String(this.total()).length, '0'),
  );

  /** The overlay's iframe, once it exists. Only set while an embed slide is up. */
  private embedFrame: HTMLIFrameElement | null = null;

  /** Resolves when the framed app has bootstrapped and can be navigated. */
  private embedReady: Promise<void> | null = null;

  private driveController: AbortController | null = null;

  /** Bumped on every cancelDrive(); only the newest navigation may start a drive. */
  private navToken = 0;

  private autoplayTimer: ReturnType<typeof setInterval> | null = null;

  private static readonly AUTOPLAY_MS = 6000;

  private static readonly EMBED_TIMEOUT_MS = 5000;

  constructor() {
    document.addEventListener('fullscreenchange', () => {
      this.browserFullscreen.set(!!document.fullscreenElement);
    });
  }

  /**
   * Handed the overlay's iframe when it enters the DOM, and null when it leaves.
   * The element is the overlay's to render and the service's to drive, so this is
   * the seam between them.
   */
  registerEmbedFrame(frame: HTMLIFrameElement | null): void {
    if (frame === this.embedFrame) return;
    this.embedFrame = frame;
    // A frame that just appeared has no app in it yet, and one that went away
    // takes its readiness with it. Loading is showEmbed()'s job, so that the
    // slide's drive can wait for the same promise.
    this.embedReady = null;
  }

  /**
   * Reads present/tour/slide from the current URL and starts the walkthrough.
   * The demo build presents by default; pass `?present=0` to open the plain console.
   * Without a `tour` param it opens the default tour at its first slide; the chooser
   * is reached from there via "Naar de keuze". `?tour=<id>` deep-links into a tour.
   */
  initFromUrl(): void {
    const params = new URLSearchParams(window.location.search);
    // Resolve the locale before the present=0 bail-out, so the signal is correct
    // even when the walkthrough is switched off.
    this.locale.set(resolveLocale(params.get('lang')));
    if (params.get('present') === '0') return;
    const tourId = params.get('tour');
    if (!tourId) {
      this.startTour(DEFAULT_TOUR_ID);
      return;
    }
    this.startTour(tourId, PresentationService.parseSlideParam(params.get('slide')));
  }

  /**
   * `?slide=` as a zero-based index. Deck links are shared and hand-edited, so a
   * missing, truncated or non-numeric value must fall back to the first slide —
   * NaN would propagate through goto()'s clamp and leave the deck blank.
   */
  private static parseSlideParam(value: string | null): number {
    const parsed = Number.parseInt(value ?? '', 10);
    return Number.isFinite(parsed) ? Math.max(1, parsed) - 1 : 0;
  }

  /** Resolves one localized string in the current locale. */
  text(value: Localized): string {
    return value[this.locale()];
  }

  toggleLocale(): void {
    this.setLocale(this.locale() === 'nl' ? 'en' : 'nl');
  }

  /**
   * Switching language only re-renders the narration panel. It deliberately does
   * not go through the router: the app pane must not re-navigate and the current
   * slide's drive script must not run a second time.
   */
  setLocale(locale: Locale): void {
    if (locale === this.locale()) return;
    this.locale.set(locale);
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    this.applyTitle();
    // Patch `lang` in place so a reload or a copied link keeps the language,
    // without a history entry per toggle.
    const url = new URL(window.location.href);
    url.searchParams.set('lang', locale);
    window.history.replaceState(window.history.state, '', url);
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
    this.toasts.dismiss();
    this.active.set(true);
    this.mode.set('chooser');
    this.applyClasses();
    this.applyTitle();
    // Drop tour/slide so a reload lands on the chooser again.
    this.router.navigate([this.currentPath()], {
      queryParams: { present: 1, lang: this.locale() },
    });
  }

  goto(index: number): void {
    // An open app modal (native <dialog>) traps focus and makes the deck inert, so
    // close it before moving on — otherwise the presenter is stuck on the slide.
    closeOpenAppDialogs();
    // A toast raised by the previous slide's drive script belongs to that slide.
    // ToastService only clears on navigation, and its set-then-navigate grace
    // period is spent by the slide's own navigation, so drop it explicitly.
    this.toasts.dismiss();
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
      this.title.setTitle(this.ui().chooserTitle);
      return;
    }
    const slide = this.currentSlide();
    if (slide) this.title.setTitle(this.documentTitle(this.text(slide.title)));
  }

  /**
   * Slide titles are suffixed with the demo name, except on the opening slide of
   * the intro tour, whose title is the product name itself — "Fundament —
   * Fundament demo" would read as a mistake.
   */
  private documentTitle(slideTitle: string): string {
    const demo = this.ui().demoTitle;
    return slideTitle === DECK_NAME ? demo : `${slideTitle} — ${demo}`;
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
    while (
      i > 0 &&
      i < this.total() - 1 &&
      this.skipOptional() &&
      this.tour().slides[i].skippable
    ) {
      i += dir;
    }
    return Math.min(Math.max(0, i), this.total() - 1);
  }

  /** Toggle the browser's native fullscreen (the `f` shortcut). */
  toggleFull(): void {
    // browserFullscreen tracks document.fullscreenElement via the fullscreenchange
    // listener in the constructor.
    if (this.browserFullscreen()) {
      document.exitFullscreen().catch(() => undefined);
    } else {
      document.documentElement.requestFullscreen().catch(() => undefined);
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
    this.toasts.dismiss();
    if (document.fullscreenElement) document.exitFullscreen().catch(() => undefined);
    this.active.set(false);
    this.mode.set('chooser');
    document.documentElement.classList.remove('presenting', 'presenting-full', 'presenting-embed');
    // Hand the title back to the console; the next route change re-sets it.
    this.title.setTitle(this.ui().consoleTitle);
    this.router.navigate([this.currentPath()], { queryParams: {} });
  }

  /** The chooser covers the whole viewport, like a full-bleed slide. */
  private applyClasses(): void {
    const root = document.documentElement.classList;
    root.toggle('presenting', this.active());
    root.toggle('presenting-full', this.active() && (this.isFull() || this.mode() === 'chooser'));
    // The console stays mounted behind the frame, keeping its state for the
    // slide that comes back to it; it is only taken out of view.
    root.toggle('presenting-embed', this.active() && !!this.embedPath());
  }

  private currentPath(): string {
    // The router url carries the query string and fragment; only the path is a route.
    return this.router.url.split(/[?#]/)[0] || '/';
  }

  private syncUrlAndNavigate(): void {
    this.cancelDrive();
    // Taken after cancelDrive(), which bumps the token: this is the only
    // navigation now allowed to start a drive.
    const token = this.navToken;
    const slide = this.currentSlide();
    const queryParams = {
      present: 1,
      tour: this.tourId(),
      slide: this.index() + 1,
      lang: this.locale(),
    };
    // An embed slide leaves the console where it stands and moves the framed
    // marketplace instead, but still writes the deck's own query params so the
    // slide stays deep-linkable and survives a reload.
    const path = (slide?.embed ? undefined : slide?.route) ?? this.currentPath();
    this.router.navigate([path], { queryParams }).then(async () => {
      // A navigation that a later goto() superseded still resolves (with false),
      // so without this guard holding down → would let the abandoned slide's
      // callback cancel the current slide's drive and run its own script against
      // whatever route is on screen by then.
      if (token !== this.navToken) return;
      const doc = slide?.embed ? await this.showEmbed(slide.embed) : document;
      // Awaiting the frame gave a later goto() the chance to move on.
      if (token !== this.navToken || !doc) return;
      if (slide?.drive?.length) this.startDrive(slide, doc);
    });
  }

  /**
   * Points the framed marketplace at `path` and resolves with its document once
   * the app in it can be driven. The first embed slide loads the bundle; later
   * ones are a message the frame routes on internally, so stepping between
   * marketplace slides does not reload and reset the app.
   *
   * Resolves with null when there is no frame (the overlay has not rendered it
   * yet) or the slide is not an embed slide.
   */
  private async showEmbed(path: string): Promise<Document | null> {
    if (!path) return null;
    const frame = await this.waitForFrame();
    if (!frame) return null;
    const src = `${MARKETPLACE_EMBED_BASE}#${path}`;
    if (!this.embedReady) {
      this.embedReady = PresentationService.loadEmbed(frame, src);
    } else {
      frame.contentWindow?.postMessage(
        { type: EMBED_NAVIGATE_MESSAGE, path },
        window.location.origin,
      );
    }
    await this.embedReady;
    return frame.contentDocument;
  }

  /**
   * The overlay renders the frame off `embedPath()`, so it appears a change
   * detection cycle after the slide index moved. Navigation can win that race,
   * hence the short poll rather than reading `embedFrame` once.
   */
  private async waitForFrame(): Promise<HTMLIFrameElement | null> {
    for (let attempt = 0; attempt < 20; attempt += 1) {
      if (this.embedFrame) return this.embedFrame;
      // eslint-disable-next-line no-await-in-loop
      await new Promise((resolve) => {
        setTimeout(resolve, 50);
      });
    }
    return this.embedFrame;
  }

  /**
   * Loads the marketplace demo into `frame` and waits for it to say it has
   * bootstrapped. The iframe's own `load` fires when the HTML arrives, well
   * before Angular has rendered anything worth driving, so the frame posts a
   * ready message and this listens for it. The timeout is the fallback for a
   * frame that never reports in: a slide that shows a half-loaded app beats one
   * that hangs the deck.
   */
  private static loadEmbed(frame: HTMLIFrameElement, src: string): Promise<void> {
    const target = frame;
    return new Promise((resolve) => {
      // One exit: abort. It unregisters the message listener (which is bound to
      // the same signal), stops the timer and resolves, whether the frame
      // reported in or the timeout ran out.
      const listening = new AbortController();
      const timer = setTimeout(() => listening.abort(), PresentationService.EMBED_TIMEOUT_MS);
      listening.signal.addEventListener('abort', () => {
        clearTimeout(timer);
        resolve();
      });
      window.addEventListener(
        'message',
        (event: MessageEvent) => {
          if (event.origin !== window.location.origin) return;
          if ((event.data as { type?: string } | null)?.type !== EMBED_READY_MESSAGE) return;
          listening.abort();
        },
        { signal: listening.signal },
      );
      target.src = src;
    });
  }

  private startDrive(slide: Slide, doc: Document): void {
    this.cancelDrive();
    const controller = new AbortController();
    this.driveController = controller;
    // runDrive swallows its own errors (including aborts), so nothing to handle here.
    runDrive(slide.drive ?? [], controller.signal, doc);
  }

  private cancelDrive(): void {
    // Invalidates any in-flight navigation callback too, so leaving a slide (or
    // the tour) can never be followed by that slide's drive starting late.
    this.navToken += 1;
    this.driveController?.abort();
    this.driveController = null;
  }
}
