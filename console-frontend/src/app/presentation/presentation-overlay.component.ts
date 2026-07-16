import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { PresentationService } from './presentation.service';

@Component({
  selector: 'app-presentation-overlay',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '(document:keydown)': 'onKeydown($event)' },
  template: `
    @if (presentation.active()) {
      <aside class="deck" [class.full]="presentation.isFull()" aria-label="Presentation narration">
        <div class="too-small">
          <p>De presentatiemodus werkt het beste op een groot scherm of projector.</p>
          <button type="button" class="ghost-btn" (click)="presentation.stop()">Sluiten</button>
        </div>

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
            @if (slide.aside) {
              <p class="deck-aside">{{ slide.aside }}</p>
            }
          }
        </div>

        <div class="footer">
          <div class="footer-row">
            <div class="counter-wrap">
              <span class="counter">
                {{ presentation.currentLabel() }} <span class="sep">/</span> {{ presentation.total() }}
              </span>
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
            <span><span class="k">Esc</span> sluit</span>
            <span [class.active]="presentation.browserFullscreen()"><span class="k">f</span> volledig scherm</span>
            <span [class.active]="presentation.autoplay()"><span class="k">a</span> autoplay</span>
            <span [class.active]="presentation.skipOptional()"><span class="k">o</span> sla optionele over</span>
          </div>
        </div>

        <div class="progress"><div class="progress-fill" [style.width.%]="presentation.progress()"></div></div>
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
      .deck-aside {
        margin: 0.5rem 0 0;
        padding: 0.85rem 1rem;
        border-radius: 12px;
        background: rgba(255, 255, 255, 0.08);
        font-size: 0.9rem;
        line-height: 1.4;
        opacity: 0.85;
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
      this.presentation.stop();
      return;
    }
    if (inField) return;

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
