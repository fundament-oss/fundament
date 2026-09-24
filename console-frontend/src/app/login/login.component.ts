import '@nldd/design-system/top-navigation-bar';
import {
  Component,
  inject,
  OnInit,
  signal,
  ChangeDetectionStrategy,
  CUSTOM_ELEMENTS_SCHEMA,
} from '@angular/core';
import { ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import AutofocusDirective from '../autofocus.directive';
import { TitleService } from '../title.service';
import AuthnApiService from '../authn-api.service';
import { ConfigService } from '../config.service';
import { Handoff, resolveHandoff } from './login-handoff';
import { HOME } from '../address';
import '@nldd/design-system/password-field';

import '@nldd/design-system/banner';
import '@nldd/design-system/box';
import '@nldd/design-system/button';
import '@nldd/design-system/container';
import '@nldd/design-system/form';
import '@nldd/design-system/form-actions';
import '@nldd/design-system/form-field';
import '@nldd/design-system/page';
import '@nldd/design-system/page-footer';
import '@nldd/design-system/rich-text';
import '@nldd/design-system/simple-section';
import '@nldd/design-system/spacer';
import '@nldd/design-system/text-field';
import '@nldd/design-system/title';
import '@nldd/design-system/validation-list';
/**
 * Where logging in lands you when nothing else was asked for. The same page
 * opening the console at its root lands on, so the two cannot drift apart. An
 * address you were sent to before the login page wins over it, and the
 * organization is filled in on the way there.
 */
const DEFAULT_ROUTE = HOME;

/** What the page says when the visitor came here of their own accord. */
const DEFAULT_DESCRIPTION =
  'The platform your team manages its Kubernetes clusters and projects with.';

@Component({
  selector: 'app-login',
  imports: [ReactiveFormsModule, AutofocusDirective],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './login.component.html',
  schemas: [CUSTOM_ELEMENTS_SCHEMA],
  // The host is a flex item of nldd-app-view, so it needs a display and a share
  // of the height, or the page inside it collapses to its content.
  styles: ':host { display: block; flex: 1; min-height: 0; }',
})
export default class LoginComponent implements OnInit {
  private titleService = inject(TitleService);

  private router = inject(Router);

  private apiService = inject(AuthnApiService);

  private route = inject(ActivatedRoute);

  private configService = inject(ConfigService);

  private fb = inject(FormBuilder);

  /**
   * The surface that sent the visitor here, when one did (see login-handoff).
   * Read once: it comes off the address this page was opened at, which does
   * not change while the page is up.
   */
  private readonly handoff: Handoff | null;

  loginForm!: FormGroup;

  /** Whether the form has been submitted at least once. Before that, a field
   * that is not finished yet is not a mistake, only unfinished. */
  formSubmitted = signal(false);

  error = signal<string | null>(null);

  isLoading = signal(false);

  /**
   * The line under the heading, which says who this login is for. Not a
   * signal: the address the page was opened at decides it, so it is settled
   * before the first render and never changes.
   */
  readonly description: string;

  constructor() {
    const params = this.route.snapshot.queryParamMap;
    this.handoff = resolveHandoff(
      params.get('app'),
      params.get('path'),
      this.configService.getConfig(),
    );
    this.description = this.handoff?.description ?? DEFAULT_DESCRIPTION;

    this.titleService.setTitle('Log in');
    this.titleService.setDescription(
      'Log in - Fundament: Open-source platform for deploying and managing Kubernetes clusters with bare-metal provisioning',
    );
    this.loginForm = this.fb.group({
      email: ['', [Validators.required, Validators.email]],
      password: ['', [Validators.required]],
    });
  }

  get email() {
    return this.loginForm.get('email');
  }

  get password() {
    return this.loginForm.get('password');
  }

  getEmailError(): string {
    if (this.email?.hasError('required')) {
      return 'Email address is required';
    }
    if (this.email?.hasError('email')) {
      return 'Please enter a valid email address';
    }
    return '';
  }

  getPasswordError(): string {
    if (this.password?.hasError('required')) {
      return 'Password is required';
    }
    return '';
  }

  async ngOnInit() {
    // Check if user is already authenticated (check state first to avoid unnecessary API call)
    if (this.apiService.isAuthenticated()) {
      // Already signed in, so there is nothing to ask. Somebody arriving from
      // another surface still has to be sent back there rather than dropped on
      // the console's own default page.
      this.leave();
    }
  }

  async onSubmit(event?: Event) {
    // Prevent the native form submission triggered by the submit button.
    event?.preventDefault();
    this.formSubmitted.set(true);

    if (this.isLoading()) return;
    if (this.loginForm.invalid) {
      this.loginForm.markAllAsTouched();
      return;
    }

    this.isLoading.set(true);
    this.error.set(null);

    try {
      const { email, password } = this.loginForm.value;
      await this.apiService.login(email, password);
      this.leave();
    } catch (err) {
      this.error.set(err instanceof Error ? `Login failed: ${err.message}` : 'Login failed');
      this.isLoading.set(false);
    }
  }

  /**
   * Where a session takes the visitor. Another surface's is a page load and
   * not a navigation: it is a different origin, and it owns the cookie this
   * console just set on the parent domain rather than a route in this app.
   */
  private leave() {
    // Taken out before either way out, a hand-off included: a returnUrl a
    // console guard left behind for an abandoned login would otherwise wait for
    // the next console login and send it there instead of home.
    const returnUrl = localStorage.getItem('returnUrl') || DEFAULT_ROUTE;
    localStorage.removeItem('returnUrl');

    if (this.handoff) {
      window.location.assign(this.handoff.url);
      return;
    }

    this.router.navigateByUrl(returnUrl);
  }
}
