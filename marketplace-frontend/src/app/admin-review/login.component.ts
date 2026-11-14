import {
  Component,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
  ElementRef,
  afterNextRender,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { ActivatedRoute, Router } from '@angular/router';
import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/container';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/password-field';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/spacer';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/validation-list';
import { TitleService } from '../title.service';
import StaffSessionService from './staff-session.service';
import { safeReturnUrl } from './staff-auth.guard';

// Close enough to tell a typo from an address; the IDP decides the rest.
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/**
 * The review backoffice's login, modelled on DCIM's: reviewers sign in with
 * their Fundament staff account, which is also their DCIM account.
 */
@Component({
  selector: 'app-admin-login',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './login.component.html',
})
export default class LoginComponent {
  private router = inject(Router);

  private route = inject(ActivatedRoute);

  private session = inject(StaffSessionService);

  private emailField = viewChild<ElementRef<HTMLElement>>('emailField');

  email = signal('');

  password = signal('');

  /** Whether the form has been submitted at least once. Before that, a field
   *  that is not finished yet is not a mistake, only unfinished. */
  formSubmitted = signal(false);

  error = signal<string | null>(null);

  isLoading = signal(false);

  emailError = computed(() => {
    const email = this.email().trim();
    if (!email) return 'Email address is required';
    if (!EMAIL_PATTERN.test(email)) return 'Please enter a valid email address';
    return '';
  });

  passwordError = computed(() => (this.password() ? '' : 'Password is required'));

  constructor() {
    inject(TitleService).setTitle('Sign in');

    // Someone who is already signed in (in DCIM, or in another tab) has no
    // business here: send them on to where they were going. Unless the review
    // API has just refused that same session, which sending them on would only
    // repeat, round and round: say so instead, and leave the form to sign in
    // afresh.
    const refused = this.session.takeRefusal();
    this.session
      .ensureUser()
      .then((user) => {
        // A sign-in started meanwhile has taken over, and navigates itself; a
        // second navigation would cancel its own.
        if (!user || this.isLoading()) return;
        if (refused) {
          this.error.set(
            'You are signed in, but the review service did not accept your session. Sign in again, or ask an administrator to check the session setup if this keeps happening.',
          );
          return;
        }
        this.goOn().catch(() => {});
      })
      .catch(() => {});

    afterNextRender(() => {
      // After Lit's own render, which settles on a microtask after this hook.
      setTimeout(() => this.emailField()?.nativeElement.focus());
    });
  }

  onEmailInput(event: Event) {
    this.email.set((event.target as HTMLInputElement).value);
  }

  onPasswordInput(event: Event) {
    this.password.set((event.target as HTMLInputElement).value);
  }

  async onSubmit() {
    this.formSubmitted.set(true);
    if (this.isLoading()) return;
    if (this.emailError() || this.passwordError()) return;

    this.isLoading.set(true);
    this.error.set(null);

    try {
      await this.session.login(this.email().trim(), this.password());
    } catch (err) {
      this.error.set(err instanceof Error ? err.message : 'Signing in failed.');
      this.isLoading.set(false);
      return;
    }

    // Still here when the page did not open (its code failed to load, or the
    // guard sent the reviewer back): say so and let the form be used again,
    // rather than leaving the button spinning.
    if (!(await this.goOn())) {
      this.error.set('You were signed in, but the page could not be opened. Try again.');
      this.isLoading.set(false);
    }
  }

  /** Goes to the page the reviewer was on their way to. Resolves to whether
   *  that navigation happened. */
  private async goOn(): Promise<boolean> {
    const returnUrl = safeReturnUrl(this.route.snapshot.queryParamMap.get('returnUrl'));
    try {
      return await this.router.navigateByUrl(returnUrl, { replaceUrl: true });
    } catch {
      return false;
    }
  }
}
