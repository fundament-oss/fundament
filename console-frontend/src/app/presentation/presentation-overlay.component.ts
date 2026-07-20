import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { PresentationService } from './presentation.service';

@Component({
  selector: 'app-presentation-overlay',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(document:keydown)': 'onKeydown($event)' },
  template: `
    @if (presentation.active()) {
      <aside class="deck" [class.full]="presentation.deckFull()" aria-label="Presentation narration">
        <div class="too-small">
          <p>De presentatiemodus werkt het beste op een groot scherm of projector.</p>
          <button type="button" class="ghost-btn" (click)="presentation.stop()">Sluiten</button>
        </div>

        @if (presentation.mode() === 'chooser') {
          <div class="chooser">
            <header class="chooser-head">
              <h1 class="deck-title">Fundament</h1>
              <p class="deck-lead">
                Kies een rondleiding, of bekijk het platform door de ogen van een rol.
              </p>
            </header>

            <section>
              <h2 class="section-label">Verhalen</h2>
              <ul class="cards">
                @for (tour of presentation.storyTours; track tour.id) {
                  <li>
                    <button type="button" class="card story" (click)="presentation.startTour(tour.id)">
                      <svg class="card-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path [attr.d]="tour.icon" />
                      </svg>
                      <span class="card-name">{{ tour.title }}</span>
                      <span class="card-blurb">{{ tour.lead }}</span>
                    </button>
                  </li>
                }
              </ul>
            </section>

            <section>
              <h2 class="section-label">Of word een rol</h2>
              <ul class="cards">
                @for (tour of presentation.personaTours; track tour.id) {
                  <li>
                    <button type="button" class="card" (click)="presentation.startTour(tour.id)">
                      <svg class="card-icon" viewBox="0 0 24 24" aria-hidden="true">
                        <path [attr.d]="tour.icon" />
                      </svg>
                      <span class="card-name">{{ tour.persona?.name }}</span>
                      <span class="card-role">{{ tour.persona?.role }}</span>
                      <span class="card-blurb">{{ tour.persona?.blurb }}</span>
                    </button>
                  </li>
                }
              </ul>
            </section>

            <p class="chooser-aside"><span class="k">Esc</span> sluit de presentatie.</p>
          </div>
        } @else {

        <div class="deck-body">
          @if (presentation.currentSlide(); as slide) {
            <div class="eyebrow">{{ presentation.tour().title }}</div>
            <h1 class="deck-title">{{ slide.title }}</h1>
            @if (slide.lead) {
              <p class="deck-lead">{{ slide.lead }}</p>
            }
            @if (slide.bullets?.length) {
              <ul class="deck-bullets">
                @for (bullet of slide.bullets; track bullet) {
                  <li>{{ bullet }}</li>
                }
              </ul>
            }
            @if (slide.link) {
              <a class="slide-link" [href]="slide.link.url" target="_blank" rel="noopener">
                {{ slide.link.label || slide.link.url }}
              </a>
            }
          }
        </div>

        @if (presentation.currentSlide()?.aside; as aside) {
          <p class="deck-aside">{{ aside }}</p>
        }

        <div class="footer">
          <div class="footer-row">
            <div class="counter-wrap">
              <span class="counter">
                {{ presentation.currentLabel() }} <span class="sep">/</span> {{ presentation.total() }}
              </span>
              <button type="button" class="text-link" (click)="presentation.backToChooser()">
                ← Naar de keuze
              </button>
              <button type="button" class="text-link" (click)="presentation.goto(0)">↺ Opnieuw</button>
            </div>
            <div class="nav">
              <button
                type="button"
                class="round-btn"
                (click)="presentation.prev()"
                [disabled]="presentation.index() === 0"
                aria-label="Vorige slide"
              >
                <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M15 5l-7 7 7 7" /></svg>
              </button>
              <button
                type="button"
                class="round-btn"
                (click)="presentation.next()"
                [disabled]="presentation.index() === presentation.total() - 1"
                aria-label="Volgende slide"
              >
                <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 5l7 7-7 7" /></svg>
              </button>
            </div>
          </div>

          <div class="hints">
            <span><span class="k">Esc</span> terug naar de keuze</span>
            <span [class.active]="presentation.browserFullscreen()"><span class="k">f</span> volledig scherm</span>
            <span [class.active]="presentation.autoplay()"><span class="k">a</span> autoplay</span>
            <span [class.active]="presentation.skipOptional()"><span class="k">o</span> sla optionele over</span>
          </div>
        </div>

        <div class="progress"><div class="progress-fill" [style.width.%]="presentation.progress()"></div></div>
        }
      </aside>
    }
  `,
  styles: [
    `
      .deck {
        position: fixed;
        inset: 0 auto 0 0;
        width: 40vw;
        height: 100vh;
        box-sizing: border-box;
        z-index: 80;
        display: flex;
        flex-direction: column;
        gap: 1.5rem;
        padding: 3rem 2.75rem 1.5rem;
        color: #fff;
        background: #154273;
        box-shadow: 4px 0 24px rgba(0, 0, 0, 0.25);
        transition: width 0.5s cubic-bezier(0.22, 1, 0.36, 1);
        overflow: hidden;
      }
      .deck.full {
        width: 100vw;
        align-items: center;
        text-align: center;
      }
      .deck-body {
        flex: 1 1 auto;
        display: flex;
        flex-direction: column;
        gap: 1rem;
        min-height: 0;
        overflow-y: auto;
      }
      .deck.full .deck-body {
        justify-content: center;
        max-width: 46rem;
      }
      .eyebrow {
        text-transform: uppercase;
        letter-spacing: 0.14em;
        font-size: 0.72rem;
        font-weight: 700;
        color: rgba(255, 255, 255, 0.55);
      }
      .deck-title {
        margin: 0;
        font-size: clamp(1.9rem, 3vw, 3rem);
        font-weight: 700;
        line-height: 1.1;
      }
      .deck-lead {
        margin: 0;
        font-size: clamp(1.05rem, 1.4vw, 1.35rem);
        line-height: 1.5;
        opacity: 0.92;
      }
      /* Block layout, not flex: flex items are blockified and lose their list
         marker. Tailwind preflight also resets list-style, so set it explicitly. */
      .deck-bullets {
        margin: 0.5rem 0 0;
        padding-left: 1.25rem;
        list-style: disc outside;
      }
      .deck-bullets li {
        font-size: clamp(0.95rem, 1.2vw, 1.15rem);
        line-height: 1.45;
        opacity: 0.9;
      }
      .deck-bullets li + li {
        margin-top: 0.6rem;
      }
      .deck-bullets li::marker {
        color: rgba(255, 255, 255, 0.5);
      }
      .deck.full .deck-bullets {
        text-align: left;
      }
      .slide-link {
        align-self: flex-start;
        margin-top: 0.8rem;
        color: #fff;
        font-size: clamp(1.1rem, 1.5vw, 1.4rem);
        font-weight: 600;
        text-decoration: underline;
        text-underline-offset: 4px;
        transition: color 0.15s;
      }
      .slide-link:hover {
        color: rgba(255, 255, 255, 0.85);
      }
      .slide-link:focus-visible {
        outline: 2px solid #fff;
        outline-offset: 3px;
      }
      .deck.full .slide-link {
        align-self: center;
      }
      /* --- Chooser ------------------------------------------------------- */
      .chooser {
        flex: 1 1 auto;
        width: 100%;
        max-width: 68rem;
        min-height: 0;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        gap: 2.25rem;
        text-align: left;
      }
      .chooser-head {
        display: flex;
        flex-direction: column;
        gap: 0.6rem;
      }
      .section-label {
        margin: 0 0 0.9rem;
        text-transform: uppercase;
        letter-spacing: 0.14em;
        font-size: 0.72rem;
        font-weight: 700;
        color: rgba(255, 255, 255, 0.55);
      }
      .cards {
        display: grid;
        grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
        gap: 1rem;
        margin: 0;
        padding: 0;
        list-style: none;
      }
      .card {
        appearance: none;
        cursor: pointer;
        width: 100%;
        height: 100%;
        display: flex;
        flex-direction: column;
        gap: 0.35rem;
        padding: 1.25rem;
        text-align: left;
        font: inherit;
        color: #fff;
        background: rgba(255, 255, 255, 0.04);
        border: 1px solid rgba(255, 255, 255, 0.28);
        border-radius: 14px;
        transition: background 0.15s, border-color 0.15s, transform 0.15s;
      }
      .card:hover {
        background: rgba(255, 255, 255, 0.12);
        border-color: #fff;
        transform: translateY(-2px);
      }
      .card:focus-visible {
        outline: 2px solid #fff;
        outline-offset: 3px;
      }
      .card.story {
        border-color: rgba(255, 182, 18, 0.6);
      }
      .card-icon {
        width: 1.6rem;
        height: 1.6rem;
        margin-bottom: 0.9rem;
        fill: none;
        stroke: #a8c0dd;
        stroke-width: 1.7;
        stroke-linecap: round;
        stroke-linejoin: round;
      }
      .card.story .card-icon {
        stroke: #ffb612;
      }
      .card-name {
        font-size: 1.05rem;
        font-weight: 700;
        line-height: 1.25;
      }
      .card-role {
        font-size: 0.9rem;
        font-weight: 600;
        color: #a8c0dd;
      }
      .card.story .card-role {
        color: #ffb612;
      }
      .card-blurb {
        margin-top: 0.15rem;
        font-size: 0.92rem;
        line-height: 1.45;
        color: rgba(255, 255, 255, 0.82);
      }
      .chooser-aside {
        margin: 0;
        font-size: 0.78rem;
        color: rgba(255, 255, 255, 0.7);
      }
      .chooser-aside .k {
        color: #fff;
        font-weight: 600;
      }
      .deck-aside {
        flex: 0 0 auto;
        margin: 0;
        padding: 0.85rem 1rem;
        border-radius: 12px;
        background: rgba(255, 255, 255, 0.08);
        font-size: 0.9rem;
        line-height: 1.4;
        opacity: 0.85;
      }
      .deck.full .deck-aside {
        width: 100%;
        max-width: 46rem;
      }
      .footer {
        flex: 0 0 auto;
        display: flex;
        flex-direction: column;
        gap: 0.6rem;
        padding-top: 1.25rem;
      }
      .deck.full .footer {
        width: 100%;
        max-width: 46rem;
      }
      .footer-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
      }
      .counter-wrap {
        display: inline-flex;
        align-items: center;
        gap: 0.6rem;
      }
      .counter {
        font-family: 'JetBrains Mono', ui-monospace, 'SFMono-Regular', monospace;
        font-size: 0.95rem;
        letter-spacing: 0.04em;
        font-variant-numeric: tabular-nums;
        color: rgba(255, 255, 255, 0.85);
      }
      .counter .sep {
        opacity: 0.5;
      }
      .text-link {
        appearance: none;
        cursor: pointer;
        background: transparent;
        border: none;
        padding: 0;
        font: inherit;
        font-size: 0.85rem;
        color: rgba(255, 255, 255, 0.7);
        transition: color 0.15s;
      }
      .text-link:hover {
        color: #fff;
      }
      .text-link:focus-visible {
        outline: 2px solid #fff;
        outline-offset: 3px;
        border-radius: 4px;
      }
      .nav {
        display: inline-flex;
        gap: 0.6rem;
      }
      .round-btn {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 2.5rem;
        height: 2.5rem;
        border-radius: 999px;
        border: 1.5px solid rgba(255, 255, 255, 0.7);
        background: transparent;
        color: #fff;
        cursor: pointer;
        transition: background 0.15s, border-color 0.15s;
      }
      .round-btn svg {
        width: 1.1rem;
        height: 1.1rem;
        fill: none;
        stroke: currentColor;
        stroke-width: 2;
        stroke-linecap: round;
        stroke-linejoin: round;
      }
      .round-btn:hover:not(:disabled) {
        background: rgba(255, 255, 255, 0.18);
        border-color: #fff;
      }
      .round-btn:disabled {
        opacity: 0.35;
        cursor: default;
      }
      .round-btn:focus-visible {
        outline: 2px solid #fff;
        outline-offset: 2px;
      }
      .hints {
        display: flex;
        flex-wrap: wrap;
        gap: 1rem;
        font-size: 0.78rem;
        color: rgba(255, 255, 255, 0.7);
      }
      .hints .k {
        color: #fff;
        font-weight: 600;
      }
      .hints .active {
        color: #fff;
      }
      .progress {
        position: absolute;
        left: 0;
        right: 0;
        bottom: 0;
        height: 4px;
        background: rgba(255, 255, 255, 0.14);
      }
      .progress-fill {
        height: 100%;
        background: rgba(255, 255, 255, 0.85);
        transition: width 0.3s;
      }
      .ghost-btn {
        appearance: none;
        cursor: pointer;
        color: #fff;
        background: rgba(255, 255, 255, 0.1);
        border: 1px solid rgba(255, 255, 255, 0.25);
        border-radius: 10px;
        padding: 0.45rem 0.8rem;
        font: inherit;
        font-size: 1rem;
        line-height: 1;
        transition: background 0.15s, border-color 0.15s;
      }
      .ghost-btn:hover:not(:disabled) {
        background: rgba(255, 255, 255, 0.2);
        border-color: #fff;
      }
      .ghost-btn:focus-visible {
        outline: 2px solid #fff;
        outline-offset: 2px;
      }
      /* Small-screen fallback: hide the slide content, show the notice. */
      .too-small {
        display: none;
        flex-direction: column;
        gap: 1rem;
        align-items: flex-start;
        margin: auto;
        text-align: center;
      }
      @media (max-width: 900px) {
        .deck {
          width: 100vw;
        }
        .deck .deck-body,
        .deck .chooser,
        .deck .deck-aside,
        .deck .footer,
        .deck .progress {
          display: none;
        }
        .deck .too-small {
          display: flex;
        }
      }
    `,
  ],
})
export class PresentationOverlayComponent {
  readonly presentation = inject(PresentationService);

  onKeydown(event: KeyboardEvent): void {
    if (!this.presentation.active()) return;

    const target = event.target as HTMLElement | null;
    const tag = target?.tagName?.toLowerCase() ?? '';
    const inField =
      tag === 'input' ||
      tag === 'textarea' ||
      tag === 'select' ||
      tag.startsWith('nldd-') ||
      !!target?.isContentEditable;

    if (event.key === 'Escape') {
      // In native fullscreen the browser handles Esc by exiting fullscreen; don't
      // also close the presentation. A second Esc (no longer fullscreen) closes it.
      if (document.fullscreenElement) return;
      event.preventDefault();
      // From a tour, Esc steps back to the chooser; from the chooser it closes.
      if (this.presentation.mode() === 'tour') {
        this.presentation.backToChooser();
      } else {
        this.presentation.stop();
      }
      return;
    }
    if (inField) return;

    // The chooser is navigated by tabbing between cards, not by the slide keys.
    if (this.presentation.mode() === 'chooser') return;

    switch (event.key) {
      case 'ArrowRight':
      case ' ':
        event.preventDefault();
        this.presentation.next();
        break;
      case 'ArrowLeft':
        event.preventDefault();
        this.presentation.prev();
        break;
      case 'f':
      case 'F':
        event.preventDefault();
        this.presentation.toggleFull();
        break;
      case 'a':
      case 'A':
        event.preventDefault();
        this.presentation.toggleAutoplay();
        break;
      case 'o':
      case 'O':
        event.preventDefault();
        this.presentation.toggleSkipOptional();
        break;
      default:
        break;
    }
  }
}
