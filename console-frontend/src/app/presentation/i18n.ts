// Locale support for the walkthrough (demo build only).
//
// Only the narration panel is localized: the tour copy in tours.ts and the
// overlay's own chrome below. The console pane on the right is the real app and
// stays in its own language — translating it is a production concern, not a
// walkthrough one.
//
// Tour structure is shared between locales by design: `loc()` pairs the two
// strings in place, so routes, drive scripts and slide ids cannot drift apart
// between NL and EN, and a missing translation is a compile error.

export type Locale = 'nl' | 'en';

export type Localized = Record<Locale, string>;

export const LOCALES: readonly Locale[] = ['nl', 'en'];

export const DEFAULT_LOCALE: Locale = 'nl';

export const LOCALE_STORAGE_KEY = 'presentation-locale';

/** Pairs the Dutch and English wording of one string. */
export const loc = (nl: string, en: string): Localized => ({ nl, en });

export function isLocale(value: string | null): value is Locale {
  return value === 'nl' || value === 'en';
}

/**
 * The product name. The intro tour's opening slide is titled with it verbatim,
 * so the document title builder recognises that slide by this value.
 */
export const DECK_NAME = 'Fundament';

interface UiStrings {
  /** Accessible name of the narration panel; it carries `lang`, so translate it. */
  deckLabel: string;
  /** Accessible name of the framed marketplace demo in the app pane. */
  embedLabel: string;
  tooSmall: string;
  close: string;
  chooserLead: string;
  stories: string;
  personas: string;
  escCloses: string;
  toChoice: string;
  restart: string;
  prevSlide: string;
  nextSlide: string;
  hintBack: string;
  hintFullscreen: string;
  hintAutoplay: string;
  hintSkipOptional: string;
  hintLanguage: string;
  languageLabel: string;
  chooserTitle: string;
  consoleTitle: string;
  demoTitle: string;
}

/** The overlay's own chrome — everything that is not tour content. */
export const UI: Record<Locale, UiStrings> = {
  nl: {
    deckLabel: 'Presentatietoelichting',
    embedLabel: 'Plugin Marktplaats',
    tooSmall: 'De presentatiemodus werkt het beste op een groot scherm of projector.',
    close: 'Sluiten',
    chooserLead: 'Kies een rondleiding, of bekijk het platform door de ogen van een rol.',
    stories: 'Verhalen',
    personas: 'Usecases',
    escCloses: 'sluit de presentatie.',
    toChoice: '← Naar de keuze',
    restart: '↺ Opnieuw',
    prevSlide: 'Vorige slide',
    nextSlide: 'Volgende slide',
    hintBack: 'terug naar de keuze',
    hintFullscreen: 'volledig scherm',
    hintAutoplay: 'autoplay',
    hintSkipOptional: 'sla optionele over',
    hintLanguage: 'taal',
    languageLabel: 'Taal',
    chooserTitle: 'Fundament — kies je rondleiding',
    consoleTitle: 'Fundament Console',
    demoTitle: 'Fundament demo',
  },
  en: {
    deckLabel: 'Presentation narration',
    embedLabel: 'Plugin Marketplace',
    tooSmall: 'Presentation mode works best on a large screen or a projector.',
    close: 'Close',
    chooserLead: 'Pick a tour, or see the platform through the eyes of a role.',
    stories: 'Stories',
    personas: 'Use cases',
    escCloses: 'closes the presentation.',
    toChoice: '← Back to the menu',
    restart: '↺ Restart',
    prevSlide: 'Previous slide',
    nextSlide: 'Next slide',
    hintBack: 'back to the menu',
    hintFullscreen: 'fullscreen',
    hintAutoplay: 'autoplay',
    hintSkipOptional: 'skip optional',
    hintLanguage: 'language',
    languageLabel: 'Language',
    chooserTitle: 'Fundament — choose your tour',
    consoleTitle: 'Fundament Console',
    demoTitle: 'Fundament demo',
  },
};
